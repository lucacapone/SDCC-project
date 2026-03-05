# Architettura Gossip SDCC

## Obiettivo
Questo documento definisce il comportamento architetturale del sottosistema gossip per la propagazione dello stato aggregato tra nodi **peer-to-peer**, senza coordinatore centrale.

## Componenti principali
- `cmd/node`: bootstrap del nodo (configurazione, membership, engine gossip).
- `internal/config`: parsing/validazione configurazione YAML/JSON + override env.
- `internal/membership`: vista locale dei peer e timeout di sospetto.
- `internal/types`: DTO e identificatori condivisi (es. `NodeID`, `MessageID`, `StateVersion`, `MessageVersion`, `GossipMessage`).
- `internal/gossip`: loop round periodico e merge stato remoto (logica protocollo).
- `internal/transport`: astrazione trasporto (implementazioni concrete e test in-memory).

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

### Campi opzionali
- `metadata` (`map[string]string`, omesso se vuoto): estensioni non critiche per compatibilità futura.

### Semantica
- Il messaggio rappresenta uno snapshot parziale dello stato locale del mittente.
- `state_version` deve rappresentare esattamente la versione dello `state` serializzato nello stesso messaggio (nessun disallineamento temporale tra metadata e payload).
- `message_id`, `state.round` e `state.version_counter` condividono la stessa semantica di avanzamento del round locale per evitare off-by-one.
- Il ricevente applica merge locale con regole deterministiche (vedi sezione merge).
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
- `new_value = (local.value + remote.value) / 2` quando `remote_version > local_version`;
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

## Proprietà attese di convergenza e limiti

### Proprietà attese
- In rete stabile con peer raggiungibili e round periodici, gli stati tendono a convergere verso una banda ristretta (validato da test integrazione in-memory).
- La convergenza è decentralizzata: ogni nodo progredisce tramite scambi locali, senza orchestratore.

### Limiti noti
- **Peer instabili/down**: partizioni temporanee riducono velocità/accuratezza della convergenza globale.
- **Convergenza lenta**: fanout basso, alta latenza o ritardi nel loop aumentano il tempo di stabilizzazione.
- **Duplicati/out-of-order**: con merge attuale possono introdurre oscillazioni o drift temporaneo.
- **Assenza di anti-entropy strutturata**: in scenari avversi possono restare differenze residuali più a lungo.

## Verifica assenza coordinatore centrale
Architettura e implementazione correnti non introducono componenti di coordinamento centrale per la logica gossip:
- ogni nodo avvia round in autonomia;
- membership locale con seed/join e vista distribuita;
- scambio stato peer-to-peer.

L'unico riferimento a sistemi centralizzati resta opzionale e **solo osservabile** (es. log centralizzati in deploy), non coinvolto nelle decisioni di protocollo.
