# Architettura del sistema

## Panoramica

SDCC è un cluster di processi Go equivalenti. Non esiste un leader né un coordinatore del calcolo: ogni nodo conserva una replica locale dello stato aggregativo e della membership, contatta periodicamente altri peer e applica localmente le stesse regole deterministiche di merge. Il discovery iniziale può usare un endpoint HTTP o seed statici, ma dopo il bootstrap l'evoluzione dello stato dipende dal gossip peer-to-peer.

## Componenti di un nodo

- `cmd/node/main.go`: wiring di configurazione, identity, bootstrap, UDP, engine e observability;
- `internal/config`: default, parser YAML/JSON, override environment e validazione;
- `internal/identity`: generation persistente allocata a ogni boot;
- `internal/membership`: vista peer, stati, incarnation, timeout, prune e bootstrap;
- `internal/transport`: interfaccia `Transport`, adapter UDP e adapter noop;
- `internal/gossip`: round, selezione peer, envelope, ricezione e merge;
- `internal/aggregation`: factory e contratti numerici per `sum`, `average`, `min`, `max`;
- `internal/observability`: logger strutturato, collector e server HTTP.

Lo stato mutabile dell'engine e il cursore fanout sono protetti da mutex; transport e handler possono quindi consegnare messaggi mentre il loop periodico esegue round locali.

## Avvio e lifecycle

1. `config.Load` applica default, file, environment e validazione.
2. `identity.AllocateGeneration` incrementa atomically il contatore in `/var/lib/sdcc/generation` (o `SDCC_GENERATION_FILE`).
3. Il nodo registra se stesso `alive` con `node_id`, endpoint pubblicizzato e incarnation pari alla generation.
4. `membership.Bootstrap` prova il `join_endpoint`; se assente o fallisce, inserisce i peer statici restituiti da `DiscoveryPeers()`.
5. Vengono avviati server observability, transport UDP ed engine gossip.
6. Alla ricezione di `SIGINT`/`SIGTERM`, l'engine tenta un annuncio `leave`, registra lo snapshot finale e chiude transport e HTTP server.

La stessa generation alimenta `VersionEpoch` dello stato e `Incarnation` della membership. I volumi Compose separati per nodo la preservano tra stop/start; `down -v` la elimina e rende non garantito un rejoin con la stessa identità contro peer che conservino tombstone più recenti.

## Protocollo gossip

### Round e selezione dei peer

Il protocollo è push. A ogni tick di `gossip_interval_ms`, l'engine:

1. applica le transizioni di timeout e il prune;
2. aggiorna il contributo locale e ricalcola il risultato sui membri eleggibili;
3. incrementa round e contatore di versione;
4. filtra self, `dead`, `leave` e peer senza indirizzo;
5. ordina i target per `node_id` e indirizzo;
6. invia a una finestra circolare di dimensione `min(fanout, peer eleggibili)`.

Il cursore avanza di `fanout` posizioni: in una membership stabile con `N` target, tutti vengono coperti entro `ceil(N/fanout)` round. Non sono previsti retry automatici per singolo invio UDP fallito; i round successivi forniscono la ridondanza.

### Formato e serializzazione

`types.GossipMessage` viene codificato in JSON e contiene:

- `message_id`, `origin_node`, `sent_at`;
- `version` del contratto (`major`, `minor`);
- `state_version` (`epoch`, `counter`);
- `state`: tipo di aggregazione, stima, metadata per contributo e timestamp;
- `membership`: digest di `node_id`, indirizzo, stato, incarnation e `last_seen`;
- metadata opzionali, tra cui l'indirizzo canonico dell'origine.

Il transport consegna byte opachi all'engine. L'UDP adapter usa un socket persistente, limita la dimensione del datagramma, rispetta il context in invio e non interpreta il payload.

### Versioning, duplicati e fuori ordine

L'ordine dello stato usa la coppia `(epoch, counter)`. L'epoch cambia a ogni boot persistente della stessa `node_id`; il counter cresce nella generation corrente. `message_id` serve all'idempotenza di consegna, non all'ordinamento.

Gli ID già visti producono `duplicate_message_id`. Gli update globalmente vecchi vengono scartati per i campi non monotoni, ma i metadata CRDT-like per contributo possono ancora aggiungere informazioni. Contributi dello stesso nodo sono sostituiti soltanto da una versione maggiore; questo evita regressioni da messaggi fuori ordine. Un conflitto alla stessa versione privo di metadata sufficienti è classificato `same_version_different_payload` invece di scegliere arbitrariamente in base al tempo di arrivo.

## Membership

### Stati e precedence

Gli stati sono `alive`, `suspect`, `dead` e `leave`. La membership confronta prima l'incarnation e poi la precedence dello stato; update con incarnation inferiore non possono resuscitare un peer. I tombstone impediscono la reintroduzione obsoleta anche dopo il prune.

Un messaggio valido è anche heartbeat implicito per l'origine. L'indirizzo viene canonicalizzato soltanto da metadata affidabili, da una entry coerente nel digest o da un endpoint transport già riconosciuto; i placeholder seed `host:port` vengono promossi alla `node_id` logica senza lasciare duplicati.

### Failure detection

`membership_timeout_ms` viene tradotto in:

```text
SuspectTimeout = max(1 ms, membership_timeout_ms / 2)
DeadTimeout    = max(SuspectTimeout + 1 ms, membership_timeout_ms)
```

La configurazione impone `SuspectTimeout` strettamente maggiore del massimo intervallo atteso tra messaggi diretti, calcolato da gossip interval, fanout e numero di peer configurati. Ciò riduce falsi sospetti ma non costituisce un failure detector perfetto: ritardi oltre la soglia possono comunque produrre transizioni temporanee.

### Join, leave e rejoin

Il client join invia una richiesta HTTP a `http://<join_endpoint>/join` e usa la vista ricevuta; il repository include il client e test con un server join simulato, ma il cluster Compose standard non esegue un servizio discovery centrale e usa seed DNS statici. Il leave volontario viene propagato via gossip. Un processo riavviato riusa la stessa `node_id`, incrementa la generation persistente e prevale su stato/tombstone precedenti.

## Aggregazione distribuita

Ogni stato conserva contributi e versioni per nodo:

- `sum`: mappa `node_id → valore`, somma saturata e flag `overflowed`;
- `average`: mappa `node_id → (sum, count)`; il contributo locale originario è separato dalla stima corrente per evitare drift;
- `min` e `max`: mappe di valori per nodo con compatibilità per payload legacy privi di metadata.

Le mappe vengono unite per chiave e versione. Il risultato esposto viene ricalcolato usando soltanto le `node_id` logiche che la membership considera `alive`; contributi di nodi `suspect`, `dead` o `leave` restano nei metadata, ma non nel valore. In questo modo crash e rejoin possono produrre le stime corrette senza perdere informazione utile.

Tutti i nodi di una stessa esecuzione devono usare lo stesso tipo di aggregazione. Il protocollo non realizza più aggregazioni simultanee nello stesso nodo.

## Convergenza e fault tolerance

Con membership stabile, delivery ripetuta e assenza di nuovi contributi, le mappe versionate convergono allo stesso insieme di contributi e quindi allo stesso risultato. Duplicati e riordinamento non alterano l'esito. La perdita di singoli datagrammi viene tollerata dai round successivi.

Se un nodo si arresta, i peer superstiti continuano a scambiarsi stato e, dopo la failure detection, escludono il contributo del nodo. Al restart, una generation superiore permette il rejoin e la reinclusione del contributo. Partizioni temporanee possono produrre viste e stime divergenti; dopo la riconnessione, gossip e regole monotone consentono la riconvergenza.

## Scalabilità ed elasticità

Il costo di invio per nodo e per round è limitato dal fanout, ma il payload contiene stato aggregativo e digest membership completi: dimensione del messaggio, memoria e tempo di disseminazione crescono con il numero di nodi. Le topologie incluse coprono 3 e 6 nodi; i test in-memory arrivano a 8 nodi. Non viene dichiarata scalabilità oltre tali verifiche.

L'aggiunta e il rejoin sono supportati dal modello membership, ma nel Compose statico l'elasticità richiede configurazioni/servizi già definiti oppure un join endpoint esterno conforme al contratto.

## Osservabilità della convergenza

L'engine emette eventi `convergence_sample` allo startup, durante i round e dopo merge significativi. `scripts/cluster_convergence_report.sh` raccoglie i log e usa `cmd/convergence-chart` per generare CSV e SVG sotto `artifacts/`; l'analisi è passiva e non partecipa al calcolo.

## Limiti noti

- UDP non garantisce delivery, ordine o protezione da frammentazione; non sono implementati cifratura, autenticazione o retry per messaggio.
- Il payload completo limita la scala e non esiste compattazione distribuita dei contributi.
- Il set degli ID messaggio visti vive in memoria del processo.
- Il join endpoint è un meccanismo opzionale di bootstrap, non un servizio incluso nel deployment.
- Metriche e probe non hanno autenticazione e non sono pubblicati dall'host nei Compose forniti.
- Consenso forte, transazioni e linearizzabilità non sono obiettivi del sistema.

Vedere anche [Configuration](configuration.md), [Testing](testing.md) e [Observability](observability.md).
