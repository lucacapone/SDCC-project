# Architettura Gossip SDCC

## Obiettivo
Questo documento definisce il comportamento architetturale del sottosistema gossip per la propagazione dello stato aggregato tra nodi **peer-to-peer**, senza coordinatore centrale.

## Componenti principali
- `cmd/node`: bootstrap del nodo (configurazione, membership, engine gossip).
- `internal/config`: parsing/validazione configurazione YAML/JSON + override env.
- `internal/membership`: vista locale dei peer e timeout di sospetto.
- `internal/gossip`: tipo messaggio, stato locale, loop round periodico, merge stato remoto.
- `internal/transport`: astrazione trasporto (implementazioni concrete e test in-memory).

## Formato messaggio gossip
Il messaggio applicativo è `internal/gossip.Message` ed è serializzato in JSON.

### Campi obbligatori
1. `node_id` (`string`): identificativo univoco del nodo mittente.
2. `round` (`uint64`): versione logica locale del mittente al momento dell'invio.
3. `aggregation_type` (`string`): tipo aggregazione associata allo stato (`sum`, `average`, `min`, `max`).
4. `value` (`float64`): valore numerico corrente del nodo.
5. `sent_at` (`timestamp`): timestamp UTC di emissione.

### Campi opzionali
- `metadata` (`map[string]string`, omesso se vuoto): estensioni non critiche per compatibilità futura.

### Semantica
- Il messaggio rappresenta uno snapshot parziale dello stato locale del mittente.
- Il ricevente applica merge locale con regole deterministiche (vedi sezione merge).
- Il formato resta *forward-compatible* tramite `metadata` opzionale.

### Serializzazione
- Encoder/decoder: `encoding/json`.
- `sent_at` è serializzato nel formato standard JSON di `time.Time` (RFC3339/RFC3339Nano in UTC).
- Il payload trasportato è `[]byte` JSON su canale di trasporto astratto.

## Strategia di versioning dello stato
La versione logica dello stato è basata su `round`.

### Regole
1. Ogni round locale completato incrementa `State.Round` di 1.
2. Ogni merge remoto, nello stato attuale, incrementa `State.Round` di 1.
3. `round` è monotono locale (non decrescente per nodo).

### Regole di confronto versione
Stato attuale implementato:
- Il confronto è **best-effort**: il messaggio remoto viene fuso senza blocco esplicito su `round` minore/maggiore.
- `round` è usato principalmente come metrica di progresso locale e osservabilità.

Evoluzione prevista (raccomandata):
- introdurre confronto `msg.round` vs `local.round` per filtrare aggiornamenti palesemente obsoleti;
- mantenere una cache degli ultimi `(node_id, round)` applicati per deduplicazione forte.

## Regole di merge
Lo stato locale è `internal/gossip.State` e il merge remoto avviene tramite `ApplyRemote`.

### Regola base attuale
- `new_value = (local.value + remote.value) / 2`
- `new_round = local.round + 1`
- `updated_at = now_utc`

### Proprietà richieste
- **Idempotenza**: non pienamente garantita nello stato corrente (riapplicare lo stesso messaggio altera il valore).
- **Duplicati**: tollerati funzionalmente ma possono introdurre bias.
- **Out-of-order**: non bloccanti, ma senza filtro versione possono rallentare la convergenza.
- **Conflitti** (`aggregation_type` diverso): da gestire come messaggio non applicabile; raccomandato scarto con log warning.

### Direzione evolutiva consigliata
Per robustezza production-grade:
1. deduplica per `(node_id, round)`;
2. guardia su `aggregation_type` coerente;
3. regole merge specifiche per algoritmo (`sum`, `average`, `min`, `max`) oltre alla media placeholder corrente.

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
