# Architettura Gossip SDCC

## Obiettivo
Questo documento definisce il comportamento architetturale del sottosistema gossip per la propagazione dello stato aggregato tra nodi **peer-to-peer**, senza coordinatore centrale.

## Componenti principali
- `cmd/node`: bootstrap del nodo (configurazione, membership, engine gossip).
- `internal/config`: parsing/validazione configurazione YAML/JSON + override env (inclusi `join_endpoint`, `bootstrap_peers`, `seed_peers`).
- `internal/membership`: vista locale dei peer con stati `Alive`/`Suspect`/`Dead`/`leave`, timeout espliciti (`SuspectTimeout`, `DeadTimeout`), retention separata per tombstone (`PruneRetention`) e priorità tramite `Incarnation`.
- `internal/types`: DTO e identificatori condivisi (es. `NodeID`, `MessageID`, `StateVersion`, `MessageVersion`, `GossipMessage`).
- `internal/gossip`: loop round periodico, merge stato remoto e failure detection runtime integrata (`ApplyTimeoutTransitions`) con heartbeat implicito sul nodo origine dei messaggi gossip.
- `internal/aggregation`: contratti comuni delle aggregazioni + factory runtime con implementazioni dedicate (`sum`, `average`, `min`, `max`).
- `internal/transport`: astrazione trasporto + adapter UDP concreto con lifecycle (`Start`/`Send`/`Close`) e rispetto di `context.Context`.
- `internal/observability`: logger strutturato coerente con `log/slog`, collector di metriche aggregate a bassa cardinalità, stato lifecycle minimo del nodo e handler/server HTTP minimo con endpoint `/health`, `/ready` e `/metrics`.


## Integrazione runtime observability e lifecycle del nodo
Nel runtime reale il punto di integrazione primario resta `cmd/node/main.go`: il package `internal/node/` non è presente nel repository attuale e non è stato introdotto un ulteriore layer intermedio perché il wiring richiesto resta piccolo e coerente con il layout esistente.

Il collector di observability viene inizializzato all'avvio del processo e segue le transizioni minime:

Sul piano runtime la failure detection viene integrata direttamente dentro `internal/gossip/engine.go`: a ogni tick del round gossip l'engine applica `Membership.ApplyTimeoutTransitions(time.Now().UTC())` prima di selezionare i target, così i peer inattivi degradano automaticamente `alive -> suspect -> dead` senza supporto esterno dei test. Ogni messaggio gossip valido viene inoltre trattato come heartbeat implicito del suo `origin_node`, ma la canonicalizzazione dell'endpoint origine avviene solo con fonti affidabili: metadata `origin_addr` del messaggio, entry coerente nel digest membership, oppure `remoteAddr` transport solo se coincide con endpoint canonicali del digest. Se manca un endpoint validato, l'engine aggiorna solo `last_seen/status` del peer già noto senza introdurre alias effimeri. Le transizioni osservate vengono emesse come log strutturati con evento stabile `membership_transition`, utile per debugging e osservabilità operativa.
- `startup` subito dopo il caricamento configurazione e prima del bootstrap;
- `bootstrap_completed` dopo `membership.Bootstrap`;
- `transport_initialized` dopo la scelta/inizializzazione del transport reale o fallback `NoopTransport`;
- `engine_started` solo dopo `eng.Start(ctx)` completato con successo;
- `shutdown` quando il processo riceve il segnale di terminazione e pubblica lo snapshot finale.

Semantica degli endpoint:
- `/health`: liveness minimale del processo HTTP/runtime, restituisce sempre `200 OK` finché il processo è vivo e include il `node_state` corrente per debugging;
- `/ready`: readiness utile per Compose/debug locale, restituisce `503` finché il nodo non ha completato bootstrap e avvio engine, poi `200 OK` in stato `engine_started`;
- `/metrics`: esporta sia i contatori/gauge esistenti sia la gauge `sdcc_node_state{state=...}` per rendere osservabile la fase corrente del lifecycle.
- lo stesso `internal/observability.Collector` viene condiviso tra `cmd/node/main.go` e `internal/gossip/engine.go`, così round locali e merge remoti aggiornano in tempo reale `sdcc_node_rounds_total`, `sdcc_node_remote_merges_total`, `sdcc_node_known_peers` e `sdcc_node_estimate` usando lo stato runtime effettivo del nodo.

## Layer transport astratto e confini con gossip
L'engine gossip è isolato dal protocollo di rete concreto: usa solo l'interfaccia `Transport` (`Start`, `Send`, `Close`) con payload `[]byte` e destinazione `string`.

Confine architetturale implementato:
- **Dentro `internal/gossip`**: serializzazione/deserializzazione `GossipMessage`, merge stato e membership, scheduling dei round.
- **Dentro adapter `internal/transport`**: I/O rete, gestione socket e semantica lifecycle del canale di trasporto.
- **Contratto tra i due layer**: `MessageHandler` riceve solo bytes già consegnati dal transport; gossip non accede a dettagli UDP (`net.PacketConn`, dial/listen, deadline dirette).

Adapter concreto corrente (`UDPTransport`):
- `Start(ctx, handler)` apre `ListenPacket` UDP una sola volta, valida `ctx`/`handler` e avvia un read loop cancellabile.
- `Send(ctx, addr, payload)` usa in priorità la socket locale persistente aperta da `Start` (porta sorgente stabile del nodo) e mantiene fallback one-shot `DialContext` solo se la socket non è disponibile.
- `Close()` è idempotente (`sync.Once`), chiude la socket e aspetta la fine delle goroutine (`WaitGroup`).

Regole timeout/retry/lifecycle effettivamente implementate:
- **Timeout ricezione**: read loop con `SetReadDeadline(250ms)` per poter verificare periodicamente `ctx.Done()`/stato `closed`.
- **Timeout invio**: se il `context` ha deadline, viene applicata su `SetWriteDeadline`; senza deadline non esiste timeout applicativo aggiuntivo.
- **Retry**: nessun retry automatico nel transport o nell'engine (`Send` viene invocato una volta per peer nel round corrente).
- **Lifecycle**: `Start` non può essere chiamato due volte; `Close` può essere chiamato più volte; `Send` su transport chiuso restituisce errore.

## Modello concorrente runtime (lock-protected vs single-writer)

Il modello concorrente implementato nel repository separa esplicitamente i dati condivisi protetti da lock dalle porzioni usate con assunzione single-writer.

Strutture lock-protected:

- `internal/membership.Set`: protetto da `sync.RWMutex` (`mu`), copre tutte le operazioni su `peers` e `prunedWatermarks` (`Upsert`, `Touch`, `LeaveAt`, `Snapshot`, `ApplyTimeoutTransitions`, `Prune`).
- `internal/gossip.Engine.State`: protetto da `sync.Mutex` (`e.mu`) durante merge remoto e preparazione del round locale (`applyRemote`, avanzamento round/versione, snapshot stato serializzato).
- Transport spy/fake nei test di contratto: protetti con mutex locali per mantenere deterministicità delle asserzioni su handler e payload.

Assunzioni single-writer residue (esplicite):

- Nel runtime normale il loop periodico (`Engine.loop`) è l’unico writer intenzionale del round locale; i test possono invocare `RoundOnce` direttamente per stress concorrente, ma la coerenza dello stato applicativo resta garantita dal lock `e.mu`.
- `RoundTicker` viene consumato da una sola goroutine di loop; non esiste un secondo scheduler interno concorrente.
- L’inizializzazione wiring (`cmd/node/main.go`) resta single-writer in fase bootstrap/startup (config, creazione set membership, costruzione engine/transport).

Conseguenze pratiche:

- Le API membership sono safe per uso concorrente multi-goroutine.
- Le invarianti di merge gossip (`incarnation` monotona e deduplica alias/canonico) sono progettate per restare stabili anche con delivery remota concorrente ai round.

## Selezione fanout deterministica
Il round gossip ordinario in `internal/gossip/engine.go` applica il fanout dopo la failure detection (`ApplyTimeoutTransitions`), la prune e il filtro dei peer non raggiungibili (`dead`/`leave`). La politica corrente non usa più un campionamento completamente random sull'intera membership eleggibile:

1. copia i peer eleggibili e li ordina stabilmente per `node_id` e poi per `addr`, così l'ordine non dipende dall'iterazione interna delle mappe;
2. mantiene nell'engine un cursore di rotazione tra round;
3. seleziona una finestra circolare di dimensione `fanout` a partire dal cursore;
4. avanza il cursore di `fanout` posizioni per il round successivo;
5. se `fanout >= peer eleggibili`, invia a tutti i target e azzera la necessità pratica di rotazione.

Con membership stabile, N peer eleggibili e fanout F, ogni peer viene quindi selezionato almeno una volta entro `ceil(N/F)` round. Per esempio, con 5 peer e `fanout=3`, le finestre attese sono `peer1,peer2,peer3`, poi `peer4,peer5,peer1`, poi `peer2,peer3,peer4`. La scelta resta decentralizzata e locale: ogni nodo mantiene il proprio cursore, senza coordinatore centrale o stato condiviso.

## Modello membership locale
Ogni nodo mantiene una vista locale (`internal/membership.Set`) composta da record `Peer` con:

- `node_id`: identificativo logico del peer, stabile e distinto dall'indirizzo di rete.
- `addr`: endpoint di rete noto localmente nel formato `host:port`.
- `status`: stato corrente (`alive`, `suspect`, `dead`, `leave`).
- `incarnation`: versione monotona del peer usata per ordinare aggiornamenti concorrenti.
- `last_seen`: ultimo timestamp utile a timeout e osservabilità.

Transizioni principali implementate:

1. `Join`/`Upsert` inseriscono o aggiornano un peer in stato `alive`; nel bootstrap seed-only un placeholder iniziale può usare temporaneamente `host:port` come chiave finché il peer remoto non propaga il vero `node_id`.
2. `ApplyTimeoutTransitions` degrada `alive -> suspect -> dead` in base a timeout configurabili, ma salta sempre il nodo locale (`selfNodeID`) così da evitare falsi timeout auto-indotti.
   Il wiring runtime in `cmd/node/main.go` usa `membership.NewSetWithConfig(cfg.MembershipConfig())`, quindi il parametro esterno `membership_timeout_ms` viene tradotto esplicitamente in `SuspectTimeout` e `DeadTimeout` invece di lasciare i default interni del package.
3. `dead` e `leave` non restano nella membership attiva per sempre: vengono mantenuti come tombstone locali per una retention temporanea separata (`PruneRetention`) così da poter propagare la rimozione via gossip.
4. `Prune(now)` rimuove fisicamente dalla membership attiva i peer `dead`/`leave` il cui `last_seen` ha superato la retention; prima della cancellazione registra un watermark locale minimale (`node_id`, `addr`, `status`, `incarnation`, `last_seen`) usato per rifiutare digest gossip obsoleti con `incarnation` non strettamente più nuova.
5. aggiornamenti con `incarnation` più alta riattivano il peer e sovrascrivono stati precedenti, anche dopo una prune precedente; un digest con la stessa `incarnation` di un tombstone già potato non può reintrodurre il peer.
6. L'eleggibilità per il calcolo locale di `sum`, `average`, `min` e `max` deriva da una policy unica in `internal/membership.IsAggregationEligible`: contribuiscono solo peer con `status=alive`; `suspect`, `dead`, `leave` e stati vuoti/non riconosciuti restano nei metadata ma sono esclusi dal valore `state.value`. Il nodo locale è incluso se risulta `alive` nello snapshot oppure se non è ancora presente nella membership, così il bootstrap non perde il contributo self. I placeholder seed-only con `node_id` nel formato endpoint `host:port` non diventano mai chiavi eleggibili per il calcolo aggregativo: dopo il primo heartbeat/digest gossip attendibile vengono promossi al `node_id` logico canonico, e `average` filtra comunque eventuali contributi storici indicizzati da endpoint.
7. Il ricalcolo di `state.value` è sempre agganciato a uno snapshot membership esplicito: nei round locali avviene dopo `ApplyTimeoutTransitions`, `Prune` e `Snapshot`; nei merge remoti avviene dopo `markPeerAlive` e dopo il merge del digest membership ricevuto. Questo mantiene intatte le mappe dei contributi per rejoin/recovery, ma fa sì che log e metriche espongano la stima derivata dalla vista membership corrente.

## Separazione tra stato gossip, membership locale e risultato esposto

Il cambio di comportamento va letto come una separazione intenzionale tra tre livelli che non devono essere confusi:

1. **Stato gossip conosciuto**: `state.aggregation_data.*` conserva i contributi/versioni appresi per nodo (`sum`, `average`, `min`, `max`) anche quando un peer diventa `suspect`, `dead` o `leave`. Questo livello rappresenta la conoscenza CRDT-like propagata via gossip e serve a gestire duplicati, out-of-order, recovery e rejoin senza perdere contributi storici.
2. **Stato membership locale**: `internal/membership.Set` mantiene la vista locale dei peer con `status`, `incarnation`, `last_seen`, timeout e tombstone. Questa vista decide l'eleggibilità operativa tramite `IsAggregationEligible(status)`: solo `alive` è eleggibile; `suspect`, `dead` e `leave` restano stati noti ma non contribuiscono al calcolo osservabile.
3. **Risultato aggregato esposto**: `state.value`, la metrica `sdcc_node_estimate` e i log di round/merge non sono la somma grezza di tutto lo stato gossip conosciuto; sono il risultato ricalcolato usando solo i contributi dei nodi eleggibili secondo la membership locale corrente. Di conseguenza un contributo può rimanere in `aggregation_data` ma non apparire nel risultato finché il nodo non torna `alive` con un'incarnation valida.

Questa separazione evita cancellazioni distruttive dei metadata gossip, ma rende esplicito che l'output osservabile rappresenta il cluster attualmente vivo secondo la vista locale del nodo.

## Formato messaggio gossip
Il messaggio applicativo è `internal/types.GossipMessage` ed è serializzato in JSON.

### Campi obbligatori
1. `message_id` (`string`): identificativo univoco del messaggio gossip.
2. `origin_node` (`string`): identificativo univoco del nodo mittente.
3. `sent_at` (`timestamp`): timestamp UTC di emissione.
4. `version` (`object`): versione esplicita del contratto messaggio (`major`, `minor`).
5. `state_version` (`object`): versione dello stato (`epoch`, `counter`) usata dal merge; non contiene `node_id`, timestamp logico, `incarnation` membership o `message_id`, che restano campi separati dell'envelope/digest.
6. `state.round` (`uint64`): versione logica locale del mittente al momento dell'invio.
7. `state.aggregation_type` (`string`): tipo aggregazione associata allo stato (`sum`, `average`, `min`, `max`).
8. `state.value` (`float64`): valore numerico corrente del nodo.
9. `state.aggregation_data.sum` (`object`, opzionale): metadati minimali per `sum` idempotente (`contributions`, `versions`, `overflowed`).
10. `state.aggregation_data.average` (`object`, opzionale): metadati per `average` convergente (`contributions` con `sum/count` per nodo + `versions`); il runtime locale mantiene separatamente il valore originario del nodo, cosi' il contributo locale non viene rimpiazzato dalla stima aggregata corrente durante i round successivi.
11. `state.aggregation_data.min` (`object`, opzionale): metadati CRDT-like per `min` (`contributions` scalari per nodo + `versions`) usati per merge robusto, ricalcolo membership-aware e retrocompatibilità.
12. `state.aggregation_data.max` (`object`, opzionale): metadati CRDT-like per `max` (`contributions` scalari per nodo + `versions`) usati per merge robusto, ricalcolo membership-aware e retrocompatibilità.
13. `membership` (`array`): digest membership completo con entry (`node_id`, `addr`, `status`, `incarnation`, `last_seen`) propagato ad ogni round, escludendo esplicitamente il nodo locale.

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
La versione logica è composta da **`version_epoch` + `version_counter`** (`internal/types.StateVersionStamp`). Questa versione è locale al flusso di stato del nodo che produce il messaggio: gli identificatori `node_id`/`origin_node`, `message_id`, `sent_at` e le `incarnation` membership non fanno parte del confronto `StateVersionStamp`, ma vengono usati rispettivamente per deduplica, osservabilità, tie-break diagnostici e merge membership.

### Regole
1. Ogni round locale completato incrementa `State.Round` e `State.VersionCounter` di 1.
2. Ogni merge remoto applicato aggiorna `version_counter` con `max(local, remote)+1`.
3. `version_epoch` è mantenuto per evoluzioni future (reset/riavvii logici) e partecipa al confronto versione.
4. `round` resta presente per retrocompatibilità e osservabilità.

### Regole di confronto versione
Implementazione attuale:
- confronto lessicografico su `(version_epoch, version_counter)`;
- duplicati (`SeenMessageIDs`) vengono ignorati in modo idempotente;
- out-of-order per mittente (`LastSeenVersionByNode`) vengono scartati (`out_of_order_stale`);
- per aggregazioni prive di metadati per-nodo, messaggi con versione globale inferiore vengono scartati (`older_version`);
- per aggregazioni CRDT-like (`sum`, `average`, `min`, `max`), una versione globale uguale o inferiore può comunque trasportare contributi per-nodo non ancora osservati: il merge confronta quindi `aggregation_data.<tipo>.versions[node_id]` prima di decidere se applicare, ignorare o risolvere un tie-break locale.

## Regole di merge
Lo stato locale è `internal/types.GossipState` e il merge remoto avviene tramite `applyRemote` in `internal/gossip/state.go`.

### Regola di merge implementata
- per `sum`: merge idempotente per chiave (`node_id`) sullo stato canonico `aggregation_data.sum.contributions`; per ogni nodo vince solo il contributo con versione più recente (`aggregation_data.sum.versions[node_id]`) e, a parità di versione ma payload diverso, si applica tie-break deterministico stabile (valore numericamente maggiore);
- `estimate` per `sum` è derivato dai contributi per nodo; nei round locali il calcolo filtra i contributi usando l'eleggibilità membership (`alive`), senza cancellare le entry da `aggregation_data.sum.contributions`;
- in overflow numerico della `sum` viene applicata saturazione a `±math.MaxFloat64` e il flag `aggregation_data.sum.overflowed=true`;
- per `average`: merge CRDT-like per contributo nodo con deduplica su versione contributo (`aggregation_data.average.versions[node_id]`) e ricostruzione deterministica della media su `sum/count`; nei round locali e dopo i merge remoti la media usa solo nodi membership-eligible con contributo noto e conserva comunque tutte le entry in `aggregation_data.average.contributions`; i log `remote_merge` espongono sia il totale dei contributi average noti sia il sottoinsieme ordinato dei `node_id` realmente usati nel calcolo, così `peers` non viene confuso con la cardinalità della media; gli stessi log distinguono `aggregation_changed`/`estimate_after_aggregation_merge` dallo stadio successivo `membership_recalculation_changed`/`estimate_after_membership_recalculation`, rendendo evidente se la media cambia per nuovi contributi o per filtro membership/eligibilità; a parità di versione per lo stesso `node_id`, caso che non dovrebbe essere generato da un nodo corretto perché ogni modifica locale incrementa `version_counter`, il tie-break stabile sceglie il contributo con `sum` maggiore e poi `count` maggiore;
- per `min`: merge per-contributo nodo su `aggregation_data.min.contributions` guidato da `aggregation_data.min.versions`; a parità di versione vince deterministicamente il contributo più basso e il valore osservabile viene ricalcolato sui soli nodi membership-eligible senza cancellare contributi storici; se nessun contributore eleggibile ha contributi noti il valore derivato è `0`, salvo il normale caso di round locale in cui il contributo self eleggibile viene prima registrato;
- per `max`: merge per-contributo nodo su `aggregation_data.max.contributions` guidato da `aggregation_data.max.versions`; a parità di versione vince deterministicamente il contributo più alto e il valore osservabile viene ricalcolato sui soli nodi membership-eligible senza cancellare contributi storici; se nessun contributore eleggibile ha contributi noti il valore derivato è `0`, salvo il normale caso di round locale in cui il contributo self eleggibile viene prima registrato;
- messaggi auto-originati (`origin_node == local.node_id`, oppure fallback legacy con `origin_node` vuoto e `state.node_id == local.node_id`) sono classificati come `self_origin_noop` e non alterano `estimate`, `round` o versioni locali;
- `new_round = max(local.round, remote.round) + 1`;
- `updated_at = now_utc`;
- tracciamento `last_message_id` e `last_sender_node_id` (derivati da `message_id`/`origin_node`);
- metadati locali non serializzati: `SeenMessageIDs`, `LastSeenVersionByNode`.

### Esiti merge esposti
`applyRemote` restituisce `MergeResult` con:
- `applied`: update remoto applicato;
- `partial_merge`: il payload aggregativo remoto e' stato ignorato come skip, ma una fase runtime successiva ha modificato la stima osservabile, ad esempio per ricalcolo membership-aware dopo aggiornamento della vista peer; in questo caso `aggregation_changed=false`, `membership_recalculation_changed=true` e le due stime intermedie chiariscono che la variazione non deriva da nuovi contributi accettati;
- `skipped`: no-op (duplicato, stessa versione+payload, versione vecchia/out-of-order) con `estimate_after == estimate_before` per gli skip realmente ignorati;
- `skipped/self_origin_noop`: messaggio auto-originato scartato come no-op silenziosa (log solo a livello `DEBUG`);
- `conflict`: conflitto rilevato (es. stessa versione con payload diverso o aggregazione incompatibile).

### Risoluzione conflitti
- `aggregation_type` differente: conflitto e scarto update;
- stessa versione globale ma payload differente:
  - per `sum`, `average`, `min` e `max` è un caso valido quando payload concorrenti portano contributi di nodi diversi con lo stesso `version_counter` locale; non viene classificato come conflitto globale, ma come merge per-contributo (`remote_contribution_merged`);
  - se lo stesso `node_id` compare con identica versione contributo e valore diverso, il payload è anomalo/legacy ma viene comunque risolto deterministicamente: `sum` sceglie il contributo numericamente maggiore, `average` sceglie la coppia lessicograficamente maggiore `(sum, count)`, `min` sceglie il contributo più basso e `max` quello più alto;
  - per aggregazioni non CRDT-like resta il conflitto `same_version_different_payload`, risolto con tie-break deterministico su timestamp più recente, poi `sender_node_id`, poi `message_id`.


## Regole merge membership
Il digest `membership` viene unito localmente entry-per-entry con proprietà di convergenza in presenza di duplicati e out-of-order:

1. `incarnation` maggiore vince sempre (update obsoleto ignorato).
2. A parità di `incarnation`, prevale lo stato a priorità maggiore (`alive < suspect < dead < leave`).
3. `last_seen` e `addr` vengono aggiornati solo se il nuovo dato è più recente/non vuoto.
4. L'operazione è idempotente: riapplicare lo stesso digest non altera lo stato.
5. Le entry remote che rappresentano il nodo locale vengono ignorate esplicitamente nel merge runtime.

## Versioning membership e regole incarnation
Il versioning membership non usa contatori globali condivisi: l'ordinamento è locale per peer e si basa su `incarnation`.

- `incarnation` è il discriminante primario: update con `incarnation` inferiore non devono sovrascrivere lo stato locale.
- a parità di `incarnation` prevale la priorità di stato (`alive < suspect < dead < leave`) per garantire ordine deterministico.

## Lifecycle esplicito join/leave del nodo locale
Flusso operativo del nodo locale nel runtime corrente:

1. **Join/bootstrap**: all'avvio `cmd/node/main.go` registra il peer locale canonico (`node_id` + `advertise_addr`) e avvia il bootstrap (`join_endpoint` oppure fallback seed peer).
2. **Round gossip ordinari**: il loop periodico propaga stato applicativo + digest membership, escludendo normalmente l'entry `self`.
3. **Leave volontario orchestrato**: quando arriva `SIGTERM`/`SIGINT`, il nodo invoca `Engine.AnnounceLeave(...)` prima di chiudere il transport; l'API marca il peer locale in stato `leave`, incrementa l'`incarnation` e invia almeno un annuncio best-effort ai peer eleggibili includendo esplicitamente l'entry locale nel digest.
4. **Teardown transport**: solo dopo l'annuncio leave viene eseguito `Engine.Stop()` con chiusura ticker e transport.

Limiti temporali attesi lato protocollo:
- la **propagazione del leave** è best-effort sul primo annuncio inviato durante lo shutdown;
- un peer che riceve il digest `leave` smette di targettare il nodo uscito dal round gossip successivo (il filtro target esclude sempre `dead`/`leave`);
- la rimozione fisica dall'insieme attivo avviene entro `PruneRetention` dal `last_seen` del tombstone leave (default package: `18s`, oppure valore esplicito di runtime/test).
- `last_seen` è un attributo ausiliario: non annulla la regola principale su `incarnation`, ma aggiorna la freschezza osservabile quando più recente.

Questo schema evita dipendenze da ordering totale dei messaggi e mantiene convergenza eventuale con gossip best-effort.

## Timeout configurabili e trade-off failure detection
La failure detection membership dipende da timeout configurabili a runtime. Nel wiring reale del repository la mappatura è stabile e documentata così:

- `SuspectTimeout = max(1ms, membership_timeout_ms / 2)`
- `DeadTimeout = max(SuspectTimeout + 1ms, membership_timeout_ms)`

In questo modo il singolo parametro utente `membership_timeout_ms` controlla davvero le transizioni `alive -> suspect -> dead` osservate dal runtime: diminuendolo, il peer entra prima in `suspect` e poi in `dead`; aumentandolo, entrambe le transizioni vengono posticipate.

Trade-off principali:

- timeout più bassi: rilevazione guasti più rapida, ma rischio maggiore di false positive su jitter/latency.
- timeout più alti: maggiore stabilità della vista membership, ma tempi più lunghi per isolare nodi realmente down.
- intervallo gossip influenza indirettamente la bontà della detection: round più radi aumentano la probabilità di transizioni conservative verso `suspect`/`dead`; il fanout riduce i destinatari per round e va calibrato con l'intervallo, ma la rotazione deterministica garantisce che i peer eleggibili vengano coperti periodicamente invece di dipendere solo dal caso.

Per questo i timeout devono essere calibrati in base al profilo rete e al target operativo (reattività vs stabilità).

## Proprietà attese di convergenza e limiti

### Proprietà attese
- In rete stabile con peer raggiungibili e round periodici, gli stati tendono a convergere verso una banda ristretta (validato da test integrazione in-memory).
- La convergenza è decentralizzata: ogni nodo progredisce tramite scambi locali, senza orchestratore.

### Limiti noti
- **Peer instabili/down**: partizioni temporanee riducono velocità/accuratezza della convergenza globale.
- **Convergenza lenta**: intervallo gossip alto, latenza elevata o ritardi nel loop aumentano il tempo di stabilizzazione; anche fanout basso può rallentare la diffusione, pur mantenendo copertura periodica deterministica dei target in membership stabile.
- **Duplicati/out-of-order**: per le aggregazioni supportate (`sum`, `average`, `min`, `max`) sono mitigati da deduplica/versioning e merge monotoni per nodo; restano comunque possibili ritardi temporanei di riallineamento in reti degradate.
- **Assenza di anti-entropy strutturata**: in scenari avversi possono restare differenze residuali più a lungo.

## Verifica assenza coordinatore centrale
Architettura e implementazione correnti non introducono componenti di coordinamento centrale per la logica gossip:
- ogni nodo avvia round in autonomia;
- membership locale con bootstrap opzionale via `join_endpoint` attraverso un client HTTP concreto (`POST http://<join_endpoint>/join`) che invia `JoinRequest` e riceve `JoinResponse` con `snapshot` + `delta`; se il join fallisce oppure non è configurato, viene usato il fallback su peer statici;
- scambio stato peer-to-peer.

L'unico riferimento a sistemi centralizzati resta opzionale e **solo osservabile** (es. log centralizzati in deploy), non coinvolto nelle decisioni di protocollo.
