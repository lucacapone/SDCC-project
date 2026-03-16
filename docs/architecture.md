# Architettura Gossip SDCC

## Obiettivo
Questo documento definisce il comportamento architetturale del sottosistema gossip per la propagazione dello stato aggregato tra nodi **peer-to-peer**, senza coordinatore centrale.

## Componenti principali
- `cmd/node`: bootstrap del nodo (configurazione, membership, engine gossip).
- `internal/config`: parsing/validazione configurazione YAML/JSON + override env (inclusi `join_endpoint`, `bootstrap_peers`, `seed_peers`).
- `internal/membership`: vista locale dei peer con stati `Alive`/`Suspect`/`Dead`/`leave`, timeout espliciti (`SuspectTimeout`, `DeadTimeout`) e priorità tramite `Incarnation`.
- `internal/types`: DTO e identificatori condivisi (es. `NodeID`, `MessageID`, `StateVersion`, `MessageVersion`, `GossipMessage`).
- `internal/gossip`: loop round periodico e merge stato remoto (logica protocollo).
- `internal/aggregation`: contratti comuni delle aggregazioni + factory runtime con implementazioni dedicate (`sum`, `average`, `min`, `max`).
- `internal/transport`: astrazione trasporto + adapter UDP concreto con lifecycle (`Start`/`Send`/`Close`) e rispetto di `context.Context`.


## Layer transport astratto e confini con gossip
L'engine gossip è isolato dal protocollo di rete concreto: usa solo l'interfaccia `Transport` (`Start`, `Send`, `Close`) con payload `[]byte` e destinazione `string`.

Confine architetturale implementato:
- **Dentro `internal/gossip`**: serializzazione/deserializzazione `GossipMessage`, merge stato e membership, scheduling dei round.
- **Dentro adapter `internal/transport`**: I/O rete, gestione socket e semantica lifecycle del canale di trasporto.
- **Contratto tra i due layer**: `MessageHandler` riceve solo bytes già consegnati dal transport; gossip non accede a dettagli UDP (`net.PacketConn`, dial/listen, deadline dirette).

Adapter concreto corrente (`UDPTransport`):
- `Start(ctx, handler)` apre `ListenPacket` UDP una sola volta, valida `ctx`/`handler` e avvia un read loop cancellabile.
- `Send(ctx, addr, payload)` usa `DialContext` UDP per invio best-effort per messaggio, propagando errori di `context` o dial/write.
- `Close()` è idempotente (`sync.Once`), chiude la socket e aspetta la fine delle goroutine (`WaitGroup`).

Regole timeout/retry/lifecycle effettivamente implementate:
- **Timeout ricezione**: read loop con `SetReadDeadline(250ms)` per poter verificare periodicamente `ctx.Done()`/stato `closed`.
- **Timeout invio**: se il `context` ha deadline, viene applicata su `SetWriteDeadline`; senza deadline non esiste timeout applicativo aggiuntivo.
- **Retry**: nessun retry automatico nel transport o nell'engine (`Send` viene invocato una volta per peer nel round corrente).
- **Lifecycle**: `Start` non può essere chiamato due volte; `Close` può essere chiamato più volte; `Send` su transport chiuso restituisce errore.

## Modello membership locale
Ogni nodo mantiene una vista locale (`internal/membership.Set`) composta da record `Peer` con:

- `node_id`: identificativo logico del peer.
- `addr`: endpoint di rete noto localmente.
- `status`: stato corrente (`alive`, `suspect`, `dead`, `leave`).
- `incarnation`: versione monotona del peer usata per ordinare aggiornamenti concorrenti.
- `last_seen`: ultimo timestamp utile a timeout e osservabilità.

Transizioni principali implementate:

1. `Join`/`Upsert` inseriscono o aggiornano un peer in stato `alive`.
2. `ApplyTimeoutTransitions` degrada `alive -> suspect -> dead` in base a timeout configurabili.
3. `Leave` pubblica tombstone `leave` per preservare convergenza e prevenire resurrect implicite.
4. aggiornamenti con `incarnation` più alta riattivano il peer e sovrascrivono stati precedenti.

## Formato messaggio gossip
Il messaggio applicativo è `internal/types.GossipMessage` ed è serializzato in JSON.

### Campi obbligatori
1. `message_id` (`string`): identificativo univoco del messaggio gossip.
2. `origin_node` (`string`): identificativo univoco del nodo mittente.
3. `sent_at` (`timestamp`): timestamp UTC di emissione.
4. `version` (`object`): versione esplicita del contratto messaggio (`major`, `minor`).
5. `state_version` (`object`): versione dello stato (`epoch`, `counter`) usata dal merge.
6. `state.round` (`uint64`): versione logica locale del mittente al momento dell'invio.
7. `state.aggregation_type` (`string`): tipo aggregazione associata allo stato (`sum`, `average`, `min`, `max`).
8. `state.value` (`float64`): valore numerico corrente del nodo.
9. `state.aggregation_data.sum` (`object`, opzionale): metadati minimali per `sum` idempotente (`contributions`, `versions`, `overflowed`).
10. `state.aggregation_data.average` (`object`, opzionale): metadati per `average` convergente (`contributions` con `sum/count` per nodo + `versions`).
11. `state.aggregation_data.min` (`object`, opzionale): metadati monotoni per `min` (versioni per nodo) usati per merge robusto e retrocompatibile.
12. `state.aggregation_data.max` (`object`, opzionale): metadati monotoni per `max` (versioni per nodo) usati per merge robusto e retrocompatibile.
10. `membership` (`array`): digest membership completo con entry (`node_id`, `addr`, `status`, `incarnation`, `last_seen`) propagato ad ogni round.

### Payload gossip membership (dettaglio)
Il campo `membership` è un array di `MembershipEntry` serializzato integralmente ad ogni messaggio:

```text
membership: [
  {
    node_id: string,
    addr: string,
    status: "alive" | "suspect" | "dead" | "leave",
    incarnation: uint64,
    last_seen: timestamp
  }
]
```

Questa scelta privilegia robustezza di convergenza rispetto alla minimizzazione del payload.

### Campi opzionali
- `metadata` (`map[string]string`, omesso se vuoto): estensioni non critiche per compatibilità futura.

### Semantica
- Il messaggio rappresenta uno snapshot parziale dello stato locale del mittente.
- `state_version` deve rappresentare esattamente la versione dello `state` serializzato nello stesso messaggio (nessun disallineamento temporale tra metadata e payload).
- `message_id`, `state.round` e `state.version_counter` condividono la stessa semantica di avanzamento del round locale per evitare off-by-one.
- Il ricevente applica merge locale con regole deterministiche sullo stato applicativo e merge membership idempotente basato su `(incarnation, status_priority)`.
- Il formato resta *forward-compatible* tramite `metadata` opzionale.

### Serializzazione
- Encoder/decoder: `encoding/json`.
- `sent_at` è serializzato nel formato standard JSON di `time.Time` (RFC3339/RFC3339Nano in UTC).
- Il payload trasportato è `[]byte` JSON su canale di trasporto astratto.

## Strategia di versioning dello stato
La versione logica è composta da **`version_epoch` + `version_counter`** (`internal/types.StateVersionStamp`).

### Regole
1. Ogni round locale completato incrementa `State.Round` e `State.VersionCounter` di 1.
2. Ogni merge remoto applicato aggiorna `version_counter` con `max(local, remote)+1`.
3. `version_epoch` è mantenuto per evoluzioni future (reset/riavvii logici) e partecipa al confronto versione.
4. `round` resta presente per retrocompatibilità e osservabilità.

### Regole di confronto versione
Implementazione attuale:
- confronto lessicografico su `(version_epoch, version_counter)`;
- messaggi con versione inferiore vengono scartati (`older_version`);
- out-of-order per mittente (`LastSeenVersionByNode`) vengono scartati (`out_of_order_stale`);
- duplicati (`SeenMessageIDs`) vengono ignorati in modo idempotente.

## Regole di merge
Lo stato locale è `internal/types.GossipState` e il merge remoto avviene tramite `applyRemote` in `internal/gossip/state.go`.

### Regola di merge implementata
- per `sum`: merge CRDT-like per contributo nodo con deduplica su versione contributo (`aggregation_data.sum.versions[node_id]`) e ricostruzione deterministica tramite somma dei contributi;
- in overflow numerico della `sum` viene applicata saturazione a `±math.MaxFloat64` e il flag `aggregation_data.sum.overflowed=true`;
- per `average`: merge CRDT-like per contributo nodo con deduplica su versione contributo (`aggregation_data.average.versions[node_id]`) e ricostruzione deterministica della media su `sum/count` totali;
- per `min`: merge monotono robusto con metadati `aggregation_data.min.versions` per nodo; in caso di stato locale non inizializzato il valore remoto viene adottato deterministicamente (compatibilità messaggi legacy senza metadati);
- per `max`: merge monotono robusto con metadati `aggregation_data.max.versions` per nodo; in caso di stato locale non inizializzato il valore remoto viene adottato deterministicamente (compatibilità messaggi legacy senza metadati);
- `new_round = max(local.round, remote.round) + 1`;
- `updated_at = now_utc`;
- tracciamento `last_message_id` e `last_sender_node_id` (derivati da `message_id`/`origin_node`);
- metadati locali non serializzati: `SeenMessageIDs`, `LastSeenVersionByNode`.

### Esiti merge esposti
`applyRemote` restituisce `MergeResult` con:
- `applied`: update remoto applicato;
- `skipped`: no-op (duplicato, stessa versione+payload, versione vecchia/out-of-order);
- `conflict`: conflitto rilevato (es. stessa versione con payload diverso o aggregazione incompatibile).

### Risoluzione conflitti
- `aggregation_type` differente: conflitto e scarto update;
- stessa versione ma payload differente: conflitto con tie-break deterministico (timestamp più recente, poi `sender_node_id`, poi `message_id`).


## Regole merge membership
Il digest `membership` viene unito localmente entry-per-entry con proprietà di convergenza in presenza di duplicati e out-of-order:

1. `incarnation` maggiore vince sempre (update obsoleto ignorato).
2. A parità di `incarnation`, prevale lo stato a priorità maggiore (`alive < suspect < dead < leave`).
3. `last_seen` e `addr` vengono aggiornati solo se il nuovo dato è più recente/non vuoto.
4. L'operazione è idempotente: riapplicare lo stesso digest non altera lo stato.

## Versioning membership e regole incarnation
Il versioning membership non usa contatori globali condivisi: l'ordinamento è locale per peer e si basa su `incarnation`.

- `incarnation` è il discriminante primario: update con `incarnation` inferiore non devono sovrascrivere lo stato locale.
- a parità di `incarnation` prevale la priorità di stato (`alive < suspect < dead < leave`) per garantire ordine deterministico.
- `last_seen` è un attributo ausiliario: non annulla la regola principale su `incarnation`, ma aggiorna la freschezza osservabile quando più recente.

Questo schema evita dipendenze da ordering totale dei messaggi e mantiene convergenza eventuale con gossip best-effort.

## Timeout configurabili e trade-off failure detection
La failure detection membership dipende da timeout configurabili a runtime (es. `membership_timeout_ms` e timeout interni `SuspectTimeout`/`DeadTimeout`).

Trade-off principali:

- timeout più bassi: rilevazione guasti più rapida, ma rischio maggiore di false positive su jitter/latency.
- timeout più alti: maggiore stabilità della vista membership, ma tempi più lunghi per isolare nodi realmente down.
- intervallo gossip influenza indirettamente la bontà della detection: round più radi aumentano la probabilità di transizioni conservative verso `suspect`/`dead` (fanout configurabile previsto ma non ancora applicato nel loop runtime).

Per questo i timeout devono essere calibrati in base al profilo rete e al target operativo (reattività vs stabilità).

## Proprietà attese di convergenza e limiti

### Proprietà attese
- In rete stabile con peer raggiungibili e round periodici, gli stati tendono a convergere verso una banda ristretta (validato da test integrazione in-memory).
- La convergenza è decentralizzata: ogni nodo progredisce tramite scambi locali, senza orchestratore.

### Limiti noti
- **Peer instabili/down**: partizioni temporanee riducono velocità/accuratezza della convergenza globale.
- **Convergenza lenta**: intervallo gossip alto, latenza elevata o ritardi nel loop aumentano il tempo di stabilizzazione (fanout basso diventerà rilevante quando la selezione fanout sarà attivata).
- **Duplicati/out-of-order**: con merge attuale possono introdurre oscillazioni o drift temporaneo.
- **Assenza di anti-entropy strutturata**: in scenari avversi possono restare differenze residuali più a lungo.

## Verifica assenza coordinatore centrale
Architettura e implementazione correnti non introducono componenti di coordinamento centrale per la logica gossip:
- ogni nodo avvia round in autonomia;
- membership locale con tentativo opzionale di bootstrap via `join_endpoint`; in assenza di client join attivo (default runtime), viene usato il fallback su peer statici;
- scambio stato peer-to-peer.

L'unico riferimento a sistemi centralizzati resta opzionale e **solo osservabile** (es. log centralizzati in deploy), non coinvolto nelle decisioni di protocollo.
