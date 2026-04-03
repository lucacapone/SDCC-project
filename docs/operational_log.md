
## 2026-03-05 12:21:50 UTC
- **Descrizione task**: Creazione della roadmap di progetto richiesta, con obiettivi misurabili, Definition of Done e microtask ordinati M01–M12 comprensivi di verifica e rischi.
- **File modificati**: `ROADMAP.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho strutturato la roadmap per coprire esplicitamente tutti i blocchi richiesti (gossip design, membership, transport, due aggregazioni, configurazione, compose, test, osservabilità, documentazione/demo, deploy EC2) e ho aggiunto dipendenze/gate per rendere l'esecuzione sequenziale non ambigua.

## 2026-03-05 12:29:23 UTC
- **Descrizione task**: Riscrittura completa del README con indice e sezioni richieste (gossip, architettura, aggregazioni, configurazione, Docker Compose, test, demo rapida, nota EC2) e allineamento comandi ai file reali di scaffolding.
- **File modificati**: `README.md`, `configs/example.yaml`, `docker-compose.yml`, `docs/operational_log.md`.
- **Reasoning summary**: Ho aggiornato la documentazione operativa creando anche i file minimi mancanti (`configs/example.yaml` e `docker-compose.yml`) per garantire che i comandi mostrati nel README puntino a path realmente esistenti nel repository corrente.

## 2026-03-05 12:34:52 UTC
- **Descrizione task**: Inizializzazione scaffolding Go del progetto con entrypoint nodo, package interni principali, test placeholder compilabili e verifica compilazione base.
- **File modificati**: `go.mod`, `cmd/node/main.go`, `internal/config/config.go`, `internal/config/config_test.go`, `internal/gossip/types.go`, `internal/gossip/state.go`, `internal/gossip/engine.go`, `internal/gossip/engine_test.go`, `internal/aggregate/aggregate.go`, `internal/aggregate/aggregate_test.go`, `internal/transport/transport.go`, `internal/transport/transport_test.go`, `internal/membership/membership.go`, `internal/membership/membership_test.go`, `internal/observability/logger.go`, `internal/observability/logger_test.go`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho creato una base compilabile e modulare allineata alla roadmap (config, gossip, membership, transport, aggregate, observability), mantenendo implementazioni leggere con TODO tecnici espliciti per le fasi successive e test scaffolding per congelare i contratti iniziali.

## 2026-03-05 13:34:01 UTC
- **Descrizione task**: Introduzione configurazione esterna completa per bootstrap nodo, compose multi-nodo in `deploy/`, eliminazione fallback hard-coded in avvio e aggiornamento README operativo.
- **File modificati**: `internal/config/config.go`, `internal/config/config_test.go`, `cmd/node/main.go`, `internal/gossip/engine.go`, `internal/gossip/engine_test.go`, `configs/example.yaml`, `configs/node1.yaml`, `configs/node2.yaml`, `configs/node3.yaml`, `deploy/docker-compose.yml`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho esternalizzato i parametri critici (interval, fanout, porta, seed peers, timeout membership, aggregazioni abilitate) con parsing YAML/JSON + override env, ho rimosso nel bootstrap il fallback silente ai default in caso di errore config, e ho predisposto un compose a 3 nodi con rete condivisa e mount di file config dedicati documentando avvio e personalizzazione.

## 2026-03-05 14:01:19 UTC
- **Descrizione task**: Estensione della test strategy con unit test su config/aggregate/membership, test di integrazione gossip in-memory, scenari crash/restart e aggiornamento README con comandi standard e criteri di successo misurabili.
- **File modificati**: `internal/config/config_test.go`, `internal/aggregate/aggregate_test.go`, `internal/membership/membership_test.go`, `internal/gossip/integration_test.go`, `Makefile`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho introdotto un transport in-memory per rendere deterministici i test di convergenza e robustezza senza rete reale, rafforzato la validazione di configurazione e documentato una matrice di esecuzione test locale/Docker con soglie quantitative (delta convergenza e timeout) per accettazione riproducibile.

## 2026-03-05 14:07:22 UTC
- **Descrizione task**: Inserimento in README di matrice decisionale architetturale (transport, gossip/fanout, coppie aggregazioni, membership/discovery) con pro/contro e impatti su test/deploy EC2, chiudendo con richiesta decisionale a risposta secca senza implementazioni definitive.
- **File modificati**: `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho preparato un confronto strutturato tra opzioni architetturali alternative per permettere una decisione esplicita del human prima di introdurre scelte irreversibili nel codice, mantenendo lo scope limitato alla documentazione come richiesto.

## 2026-03-05 14:18:55 UTC
- **Descrizione task**: Applicazione delle decisioni architetturali selezionate (HTTP+JSON, gossip push-pull fanout variabile, aggregazioni sum/average/min/max) e raccomandazione operativa membership/discovery.
- **File modificati**: `internal/aggregate/aggregate.go`, `internal/aggregate/aggregate_test.go`, `internal/config/config.go`, `configs/example.yaml`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho esteso lo scaffolding delle aggregazioni includendo placeholder `min` e `max`, aggiornato i default/config di esempio per riflettere il set completo richiesto e documentato una raccomandazione bilanciata per membership basata su join endpoint con fallback seed statici.

## 2026-03-05 14:24:37 UTC
- **Descrizione task**: Pulizia e aggiornamento README dopo decisione finale su membership/discovery (opzione B) e consolidamento scelte architetturali definitive.
- **File modificati**: `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho rimosso la matrice decisionale e la richiesta di scelta non più necessarie, mantenendo solo le decisioni finali confermate e allineando la sezione aggregazioni/esempi config ai valori effettivamente richiesti.

## 2026-03-05 16:13:03 UTC
- **Descrizione task**: Creazione documentazione architetturale gossip (`docs/architecture.md`) con specifica del formato messaggio, strategia di versioning, regole di merge, proprietà/limiti di convergenza e verifica esplicita dell'assenza di coordinatore centrale.
- **File modificati**: `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho documentato lo stato implementato e la direzione evolutiva per rendere espliciti vincoli e comportamenti del protocollo gossip, includendo una sezione di verifica sull'assenza di componenti centralizzati nel piano di controllo del protocollo.

## 2026-03-05 16:25:05 UTC
- **Descrizione task**: Rifattorizzazione dei tipi condivisi creando `internal/types` e spostando i DTO gossip cross-package fuori da `internal/gossip`.
- **File modificati**: `internal/types/types.go`, `internal/gossip/engine.go`, `internal/gossip/state.go`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho centralizzato identificatori e strutture serializzabili (`NodeID`, `MessageID`, `StateVersion`, `MessageEnvelope`, `GossipMessage`, `GossipState`) nel nuovo package `internal/types`, mantenendo `internal/gossip` focalizzato sulla logica di round/merge e aggiornando la documentazione architetturale per evitare divergenze code-doc.

## 2026-03-05 16:36:06 UTC
- **Descrizione task**: Introduzione versioning composto dello stato gossip, merge remoto idempotente con deduplica/out-of-order handling, gestione esplicita conflitti e nuovi esiti merge per metriche/debug.
- **File modificati**: `internal/types/types.go`, `internal/gossip/state.go`, `internal/gossip/engine.go`, `internal/gossip/state_test.go`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho sostituito il merge best-effort con un flusso deterministico basato su `version_epoch + version_counter`, aggiungendo metadati locali allo stato (`SeenMessageIDs`, `LastSeenVersionByNode`) per deduplica e filtro aggiornamenti obsoleti, includendo il caso no-op su stessa versione+payload e tie-break riproducibile sui conflitti stessa versione payload diverso, con test dedicati sugli esiti `applied/skipped/conflict`.

## 2026-03-05 16:49:10 UTC
- **Descrizione task**: Standardizzazione del contratto messaggio gossip con ID univoco, versione messaggio esplicita e campi di tracciamento (`origin_node`, `state_version`, `sent_at`), con aggiornamento serializzazione engine e test associati.
- **File modificati**: `internal/types/types.go`, `internal/gossip/engine.go`, `internal/gossip/state.go`, `internal/gossip/state_test.go`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho sostituito il vecchio envelope con campi top-level nel `GossipMessage`, aggiunto fallback di normalizzazione in deserializzazione per compatibilità con payload legacy e introdotto sincronizzazione/serializzazione sicura dello stato nel loop gossip per evitare condivisione concorrente delle mappe di merge durante i test di integrazione.

## 2026-03-05 16:56:01 UTC
- **Descrizione task**: Aggiornamento test gossip con suite `TestMergeRules` deterministica per validare regole di merge richieste dalla roadmap (idempotenza, duplicate delivery, out-of-order, conflitto versione e convergenza logica).
- **File modificati**: `internal/gossip/state_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho consolidato i casi di merge in un unico test a subtest con fixture a timestamp fisso (senza dipendenza da clock reale) e helper dedicati per stato/messaggi, in modo da rendere riproducibile il comando target `go test ./internal/gossip -run TestMergeRules`.

## [2026-03-05 17:18:18] Task: Creazione assessment M01 su stato gossip
- **Descrizione task**: Creazione documento di assessment  con 6 sezioni richieste (stato microtask, componenti analizzati, stato implementazione, scelte architetturali, gap/problemi, verifica test) citando esplicitamente i file indicati.
- **File modificati**: , .
- **Reasoning summary**: Ho svolto un'analisi dei componenti gossip e documentazione architetturale per produrre un assessment descrittivo senza modificare codice applicativo, mantenendo tracciabilità operativa tramite append-only log.

## [2026-03-05 17:20:30] Nota di correzione log precedente
- **Descrizione task**: Correzione descrittiva del precedente inserimento log relativo a M01, in cui la presenza di backtick nel testo aveva causato espansioni shell indesiderate nella riga di riepilogo.
- **File modificati**: `docs/operational_log.md`.
- **Reasoning summary**: Ho mantenuto l'approccio append-only e registrato esplicitamente una nota correttiva senza sovrascrivere la cronologia esistente.

## [2026-03-05 17:18:37] Task: Assessment M01 (documentazione)
- **Descrizione task**: Redazione di  come assessment dello stato microtask con 6 sezioni richieste e citazione esplicita dei file gossip/architettura indicati, senza modifiche al codice.
- **File modificati**: , .
- **Reasoning summary**: Ho sintetizzato lo stato dell'implementazione attuale (stato locale, messaggi, merge, convergenza), le scelte architetturali e i gap residui in un documento di sola analisi, mantenendo tracciabilità nel log operativo.

## [2026-03-05 17:21:45] Task: Assessment M01 (documentazione)
- **Descrizione task**: Redazione di `docs/task/M01.md` come assessment dello stato microtask con 6 sezioni richieste e citazione esplicita dei file gossip/architettura indicati, senza modifiche al codice.
- **File modificati**: `docs/task/M01.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho sintetizzato lo stato dell'implementazione attuale (stato locale, messaggi, merge, convergenza), le scelte architetturali e i gap residui in un documento di sola analisi, mantenendo tracciabilità nel log operativo.

## 2026-03-05 17:22:17 UTC
- **Descrizione task**: Aggiornamento README con sezione sintetica "Protocollo gossip (M01)" su struttura messaggi, versioning, regole merge e comando di verifica dedicato.
- **File modificati**: , .
- **Reasoning summary**: Ho aggiunto una sezione breve e orientata all'uso pratico, includendo un riferimento diretto a  per i dettagli completi e mantenendo coerenza con la suite test M01 esistente.

## 2026-03-05 17:22:32 UTC
- **Descrizione task**: Nota correttiva dell'entry precedente per ripristinare l'elenco completo dei file modificati e il riferimento al documento di architettura.
- **File modificati**: README.md, docs/operational_log.md.
- **Reasoning summary**: Correzione append-only dopo espansione indesiderata dei backtick nella shell; il contenuto corretto conferma l'aggiunta della sezione "Protocollo gossip (M01)" e del link a docs/architecture.md.

## 2026-03-05 17:33:25 UTC
- **Descrizione task**: Allineamento semantica di avanzamento nel round gossip per eliminare disallineamenti tra `message_id`, `state_version` e `state` serializzato; verifica convergenza del cluster residuo.
- **File modificati**: `internal/gossip/engine.go`, `internal/gossip/engine_test.go`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho anticipato l'avanzamento di `round/version_counter` prima della costruzione del messaggio e ho usato una singola versione coerente per stato locale e payload inviato, aggiungendo un test dedicato all'allineamento versione/stato e rieseguendo i test gossip focali (merge rules + crash residual convergence) per confermare che il caso `same_version_different_payload` resti eccezionale.

## 2026-03-05 17:57:23 UTC
- **Descrizione task**: Refactor del modulo membership con modello peer esteso (`NodeID`, `Addr`, `Status`, `Incarnation`, `LastSeen`), stati espliciti `Alive/Suspect/Dead`, timeout configurabili e transizioni deterministiche con priorità su incarnation.
- **File modificati**: `internal/membership/membership.go`, `internal/membership/membership_test.go`, `internal/gossip/engine.go`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho sostituito il booleano `Suspected` con una macchina a stati deterministica basata su timeout (`SuspectTimeout`/`DeadTimeout`), introdotto merge locale coerente con regola "incarnation maggiore vince", aggiornato i test unitari per coprire transizioni e precedence, e allineato l'engine gossip al nuovo campo indirizzo `Addr`.

## 2026-03-05 18:05:07 UTC
- **Descrizione task**: Estensione bootstrap nodo con configurazione discovery (`join_endpoint`, `bootstrap_peers`), fase di join attiva all'avvio con fallback statico e test in-memory per vista membership iniziale.
- **File modificati**: `internal/config/config.go`, `internal/config/config_test.go`, `cmd/node/main.go`, `internal/membership/bootstrap.go`, `internal/membership/bootstrap_test.go`, `configs/example.yaml`, `README.md`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho introdotto una fase di bootstrap esplicita seed-only: il nodo tenta join dinamico verso endpoint configurato per ottenere snapshot/delta iniziale, e se non disponibile ricade su peer statici (`bootstrap_peers` o `seed_peers` per retrocompatibilità); ho mantenuto override env, aggiornato la documentazione e aggiunto test con transport in-memory del join per verificare discovery e fallback senza introdurre una fonte autoritativa centralizzata.

## 2026-03-05 18:11:58 UTC
- **Obiettivo:** estendere il protocollo gossip con digest membership (status/incarnation), merge robusto e test di convergenza multi-nodo.
- **Attività svolte:**
  - esteso `GossipMessage` con campo `membership` e DTO serializzabile `MembershipEntry`;
  - serializzato periodicamente il digest membership ad ogni round gossip insieme allo stato applicativo;
  - implementato merge membership lato ricezione con confronto per `incarnation` e priorità di stato, mantenendo idempotenza su duplicati/out-of-order;
  - introdotto stato tombstone `leave` nel package membership per propagare eventi di uscita e preservare convergenza;
  - aggiunti test dedicati in `internal/gossip` per serializzazione digest e convergenza membership in presenza di messaggi duplicati/out-of-order;
  - aggiornata documentazione architetturale/README per coerenza del protocollo.
- **File modificati:**
  - `internal/types/types.go`
  - `internal/gossip/engine.go`
  - `internal/gossip/membership_convergence_test.go`
  - `internal/membership/membership.go`
  - `internal/membership/membership_test.go`
  - `docs/architecture.md`
  - `README.md`
  - `docs/operational_log.md`
- **Sintesi reasoning:** scelta di digest full membership per ridurre complessità di ordering degli eventi mantenendo merge deterministico (incarnation + status priority), così da garantire convergenza anche con duplicati e riordino dei messaggi.

## 2026-03-05 18:21:18 UTC
- **Descrizione task**: Estensione suite `internal/membership` per coprire flusso M02 `JoinLeave` (bootstrap join, inattività con transizioni `Suspect`/`Dead`, cleanup con tombstone `leave`) e casi deterministici di rejoin/falso positivo senza sleep lunghi.
- **File modificati**: `internal/membership/membership_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho riscritto `TestJoinLeave` usando timestamp fissi e transizioni pilotate via `ApplyTimeoutTransitions` per verificare il percorso completo B join da bootstrap A -> suspect -> dead -> leave tombstone persistente; ho aggiunto test deterministici su rejoin con `Incarnation` maggiore e mitigazione false positive tramite gossip update `Alive`, mantenendo sincronizzazione esplicita e timeout piccoli.

## 2026-03-05 18:26:06 UTC
- **Descrizione task**: Redazione documentazione M02 (`docs/task/M02.md`), aggiornamento architettura/README su membership gossip e verifica coerenza testo↔codice evitando claim su join endpoint runtime non implementato.
- **File modificati**: `docs/task/M02.md`, `docs/architecture.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho consolidato il modello membership locale, il payload gossip membership, le regole di merge/versioning basate su `incarnation` e i trade-off dei timeout di failure detection; in README ho esplicitato stato milestone M01/M02 e comandi test membership, chiarendo che nel runtime corrente viene usato `NoopJoinClient` con fallback ai peer statici.

## 2026-03-05 19:02:16 UTC
- **Descrizione task**: Correzione disallineamento lessicale in documentazione architetturale membership (`ApplyTimeouts`/`MarkLeave`) rispetto ai nomi metodo reali nel codice (`ApplyTimeoutTransitions`/`Leave`).
- **File modificati**: `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho allineato la terminologia del documento di architettura ai simboli realmente esposti dal package `internal/membership`, in modo da mantenere coerenza documento↔implementazione senza alterare il comportamento runtime.

## 2026-03-16 16:24:31 UTC
- **Descrizione task**: Implementazione adapter `Transport` concreto su UDP con lifecycle sicuro (`Start`/`Send`/`Close`), aggiornamento wiring del nodo e allineamento documentazione.
- **File modificati**: `internal/transport/udp_transport.go`, `internal/transport/udp_transport_test.go`, `internal/transport/transport.go`, `cmd/node/main.go`, `README.md`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho introdotto `UDPTransport` per mantenere `engine` disaccoppiato dal protocollo (solo `[]byte` + indirizzo), con loop ricezione cancellabile via `context`, invio con `DialContext` e shutdown idempotente senza deadlock; ho aggiornato il bootstrap runtime per usare UDP con fallback esplicito a `NoopTransport` solo in caso di errore di inizializzazione e riallineato i documenti al comportamento reale.

## 2026-03-16 16:37:21 UTC
- **Descrizione task**: Aggiunta suite contrattuale `TestTransportContract` con fake deterministic transport e nuovi test `Engine` per verificare interazione esclusiva via interfaccia `Transport` senza dipendenza da HTTP/UDP reali.
- **File modificati**: `internal/transport/contract_test.go`, `internal/gossip/engine_transport_contract_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho modellato un transport fake in-memory deterministico per coprire scenari richiesti (delivery, context cancel/timeout, close idempotente, send-after-close) senza rete reale; lato gossip ho introdotto uno spy transport che cattura handler/start/close e consente delivery manuale del payload per verificare il wiring dell'`Engine` solo contro il contratto astratto `Transport`, evitando timing flaky.

## 2026-03-16 16:46:25 UTC
- **Descrizione task**: Redazione milestone M03 e riallineamento README/architettura sul layer transport astratto (confine gossip↔adapter), regole timeout/retry/lifecycle e verifica assenza claim non implementati.
- **File modificati**: `docs/task/M03.md`, `README.md`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho documentato esplicitamente che il contratto transport è astratto e validato da test dedicato, separando comportamento implementato (UDP Start/Send/Close con context e close idempotente) da funzionalità pianificate (fanout/retry), così da eliminare divergenze testo↔codice nella milestone M03.

## 2026-03-16 17:48:55 UTC
- **Descrizione task**: Migrazione del layer aggregazioni da `internal/aggregate` a `internal/aggregation`, introduzione implementazione concreta `sum` e aggiornamento integrazione runtime/factory per risoluzione `cfg.Aggregation == "sum"`.
- **File modificati**: `internal/aggregation/aggregation.go`, `internal/aggregation/aggregation_test.go`, `internal/aggregation/sum/algorithm.go`, `internal/gossip/state.go`, `cmd/node/main.go`, `Makefile`, `README.md`, `docs/architecture.md`, `docs/operational_log.md`, `internal/aggregate/aggregate.go` (rimosso), `internal/aggregate/aggregate_test.go` (rimosso).
- **Reasoning summary**: Ho creato un package comune con contratto `Algorithm` e factory unica, spostando `sum` in un modulo concreto dedicato e mantenendo la compatibilità runtime tramite istanziazione della factory in bootstrap nodo e merge dinamico nel dominio gossip; gli stub legacy `SumPlaceholder` e correlati sono stati eliminati con la rimozione del vecchio package per evitare duplicazioni tra path nuovo/vecchio.

## 2026-03-16 17:59:17 UTC
- **Descrizione task**: Estensione del merge gossip per `sum` idempotente con metadati per-contributo, gestione deduplica robusta su duplicati/out-of-order, saturazione overflow numerico e aggiornamento documentazione tecnica.
- **File modificati**: `internal/types/types.go`, `internal/gossip/state.go`, `internal/gossip/engine.go`, `internal/gossip/state_test.go`, `docs/architecture.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho introdotto nel payload stato un blocco dominio-specifico `aggregation_data.sum` (contributi e versioni per nodo) per separare transport e logica aggregativa, mantenendo il transport byte-oriented; il merge `sum` ora è deterministico/idempotente perché aggiorna un contributo solo se la versione remota è più recente, scarta implicitamente duplicati e messaggi riordinati, e ricostruisce il totale da mappa contributi. Ho inoltre documentato e testato il comportamento di overflow con saturazione a `±MaxFloat64` e flag di overflow propagato.

## 2026-03-16 18:08:13 UTC
- **Descrizione task**: Aggiunta suite deterministica `TestSumConvergence` per aggregazione `sum` con copertura di convergenza multi-nodo, duplicate update, out-of-order, nodo lento e overflow con policy di saturazione.
- **File modificati**: `internal/aggregation/sum/sum_convergence_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho implementato un harness con transport stub sincrono per evitare flakiness e rete reale, usando timestamp/versioni controllati e senza sleep casuali, così da verificare in modo ripetibile le proprietà di convergenza e idempotenza del merge `sum`, includendo assert espliciti su saturazione a `math.MaxFloat64` e flag overflow.

## 2026-03-16 18:13:50 UTC
- **Descrizione task**: Redazione milestone M04 su aggregazione `sum`, aggiornamento README con stato reale post-patch e inserimento comando operativo di verifica `TestSumConvergence`.
- **File modificati**: `docs/task/M04.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho consolidato la documentazione M04 con stato iniziale/gap/regole merge-update/duplicati-overflow/test, esplicitando in README che `sum` è implementata con payload `aggregation_data.sum`, deduplica idempotente e convergenza verificabile via comando mirato; l'allineamento resta coerente con `docs/architecture.md` su payload e regole di convergenza.

## 2026-03-16 19:32:34 UTC
- **Task:** Introduzione package aggregazione dedicati (`average`, `min`, `max`) e semantica gossip convergente reale per `average`.
- **Files modified:**
  - `internal/aggregation/aggregation.go`
  - `internal/aggregation/average/algorithm.go`
  - `internal/aggregation/min/algorithm.go`
  - `internal/aggregation/max/algorithm.go`
  - `internal/types/types.go`
  - `internal/gossip/engine.go`
  - `internal/gossip/state.go`
  - `internal/gossip/state_test.go`
  - `README.md`
  - `docs/architecture.md`
- **Reasoning summary:**
  - Rimossi gli stub dal factory runtime e sostituiti con package dedicati che mantengono la stessa API `Type()/Merge(...)`.
  - Per `average` è stata introdotta una semantica convergente basata su contributi per nodo (`sum/count`) con versioning per-contributo, deduplica e merge idempotente/out-of-order-safe nel layer gossip.
  - Mantenuta la compatibilità dell'interfaccia aggregazione e della selezione runtime via configurazione.
  - Aggiornati test e documentazione per riflettere la nuova semantica distribuita dell'average.

## 2026-03-16 19:43:22 UTC
- **Descrizione task**: Estensione merge gossip per metadati opzionali `average/min/max` con retrocompatibilità completa dei messaggi legacy e robustezza monotona per `min`/`max`.
- **File modificati**: `internal/types/types.go`, `internal/gossip/state.go`, `internal/gossip/state_test.go`, `docs/architecture.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho esteso il modello `AggregationState` con stati opzionali `Min/Max` (mappe versioni per nodo), mantenendo `Average` convergente e implementando rami espliciti in `mergeAggregationState` per `average/min/max`. Per `min`/`max` il merge ora gestisce in modo deterministico i casi di stato locale vuoto/peer legacy non inizializzati e usa metadati monotoni per evitare regressioni su update vecchi. Ho inoltre allineato le funzioni di normalizzazione `ensureIncoming...Metadata` e aggiunto test mirati per validare compatibilità backward con messaggi senza nuovi metadati.

## 2026-03-16 19:52:59 UTC
- **Descrizione task**: Aggiunta suite di convergenza per `average`, `min`, `max`, regressione `sum` in gossip e rafforzamento test Factory/config per nuove aggregazioni concrete.
- **File modificati**: `internal/aggregation/average/average_convergence_test.go`, `internal/aggregation/min/min_convergence_test.go`, `internal/aggregation/max/max_convergence_test.go`, `internal/gossip/sum_regression_test.go`, `internal/aggregation/aggregation_test.go`, `internal/config/config_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho replicato l'approccio deterministico già usato per `sum` con transport stub sincrono e versioni controllate, coprendo scenari di convergenza multi-nodo, duplicate update, out-of-order e nodo lento; per `average` ho incluso anche edge case su divisione per zero/stato vuoto. Ho inoltre aggiunto una regressione dedicata sul merge `sum` per garantire che l'estensione con nuove aggregazioni non alteri la semantica esistente, e ho esteso i test di factory/config per verificare sia i tipi concreti restituiti sia l'accettazione di `min`/`max` nella configurazione valida.

## 2026-03-16 20:24:37 UTC
- **Descrizione task**: Chiusura documentale milestone M05 con nuovo task file, aggiornamento README (stato milestone, aggregazioni supportate, configurazione e comandi test M05) e verifica/allineamento coerenza architetturale su merge, serializzazione stato e convergenza.
- **File modificati**: `docs/task/M05.md`, `README.md`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho effettuato un consolidamento documentale senza toccare il runtime perché le implementazioni `average/min/max` e la regressione multi-aggregazione erano già presenti e testate; ho esplicitato i comandi di validazione M05 richiesti e corretto la sezione limiti in architettura per riflettere che duplicati/out-of-order sono gestiti in modo robusto per le aggregazioni effettivamente supportate, preservando comunque i limiti di convergenza in reti degradate.

## 2026-03-18 00:00:00 UTC
- **Descrizione task**: Rafforzamento parsing configurazione file per distinguere default coperti, valori presenti ma invalidi e formati non supportati; aggiunta test regressione su YAML/JSON malformati.
- **File modificati**: `internal/config/config.go`, `internal/config/config_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho reso esplicita la precedence `Default() -> file -> env -> Validate()` durante il caricamento, eliminando i fallback silenziosi dei campi numerici da file e propagando errori chiari su interi non validi, liste YAML malformate/item vuoti e formati file non supportati. Ho inoltre esteso la suite `internal/config` con casi mirati per `node_port: abc`, liste peer YAML malformate, estensioni non supportate e tipi JSON incompatibili, preservando la compatibilità degli override via environment.
## 2026-03-18 17:05:57 UTC
- **Descrizione task**: Rafforzamento della validazione configurazione con controlli espliciti su host/porte, peer list e aggregazioni supportate; aggiunta test mirati per casi invalidi.
- **File modificati**: `internal/config/config.go`, `internal/config/config_test.go`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho esteso `Validate` per rifiutare porte fuori range, `bind_address` vuoti o malformati, endpoint peer non conformi a `host:porta`, liste con valori vuoti/duplicati e aggregazioni fuori dal set supportato, mantenendo messaggi d’errore espliciti e copertura test focalizzata sui nuovi edge case.


## 2026-03-18 17:11:57 UTC
- **Descrizione task**: Creazione di `docs/configuration.md` come documento canonico della configurazione runtime, allineato a codice e file di esempio.
- **File modificati**: `docs/configuration.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho documentato il comportamento reale del loader di configurazione usando come fonti `internal/config/config.go`, `cmd/node/main.go` e `configs/*.yaml`, esplicitando formati supportati, precedence `Default() -> file -> env -> Validate()`, campi della struct `Config`, default, variabili ambiente, regole di validazione ed esempi locali/override env senza introdurre divergenze rispetto al runtime implementato.

## 2026-03-18 17:12:20 UTC
- **Descrizione task**: Allineamento README al nuovo documento canonico della configurazione runtime.
- **File modificati**: `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho aggiunto nel README un riferimento esplicito a `docs/configuration.md` per evitare divergenze tra documentazione introduttiva e documento canonico, mantenendo invariato il comportamento del runtime.

## 2026-03-18 17:20:00 UTC
- **Descrizione task**: Estensione esplicita della suite `internal/config/config_test.go` per copertura M06 su parsing, default, override env, errori bloccanti e validazione principale con `TestValidateConfig`.
- **File modificati**: `internal/config/config_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho riorganizzato la suite dei test di configurazione introducendo casi espliciti per parsing YAML/JSON validi, configurazioni parziali con default, mismatch di tipo bloccanti, override via environment, messaggi d’errore leggibili con riferimento al campo, edge case su peer list e aggregazioni non supportate. Ho inoltre mantenuto `TestValidateConfig` come entry point principale per la validazione richiesta dal task, così che il comando mirato `go test ./internal/config -run TestValidateConfig` eserciti davvero i controlli bloccanti principali.
## 2026-03-18 17:35:00 UTC
- **Descrizione task**: Riorganizzazione della suite di validazione configurazione per esporre il punto d’ingresso richiesto `TestValidateConfig` con copertura esplicita dei casi principali.
- **File modificati**: `internal/config/config_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho consolidato la validazione in una singola suite `TestValidateConfig` strutturata a subtest, includendo default validi, campi obbligatori mancanti, valori numerici pericolosi, aggregazioni non supportate o non abilitate e verifiche sui messaggi d’errore leggibili; ho inoltre rimosso la duplicazione coperta dalla vecchia suite `TestValidateFailures`.

## 2026-03-18 17:33:11 UTC
- **Task**: Creazione del documento milestone M06 sulla configurazione esterna e validazione robusta.
- **File modificati**: `docs/task/M06.md`, `docs/operational_log.md`.
- **Sintesi**: Analizzato lo stato reale di `internal/config/config.go`, `internal/config/config_test.go`, `cmd/node/main.go`, `docs/configuration.md` e `README.md`; aggiunto il documento `docs/task/M06.md` per consolidare strategia di configurazione, validazioni, test eseguiti e limiti residui senza alterare il comportamento runtime.

## 2026-03-19 08:50:56 UTC
- **Descrizione task**: Promozione del file Compose di root a sorgente canonica del cluster locale multi-nodo, con allineamento della documentazione e dismissione del duplicato operativo in `deploy/`.
- **File modificati**: `docker-compose.yml`, `deploy/docker-compose.yml`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho sostituito il vecchio compose placeholder alla root con una definizione reale a tre nodi basata sul precedente file in `deploy/`, usando la rete stabile `sdcc-net`, mount espliciti delle configurazioni `configs/node(1, 2, 3).yaml` e peer Compose risolvibili (`node1`, `node2`, `node3`). Per evitare doppia sorgente di verità, il file in `deploy/` è stato ridotto a promemoria documentale che rimanda al file canonico di root; in parallelo ho aggiornato il README ai comandi standard `docker compose ...` senza flag `-f`.

## 2026-03-19 08:51:28 UTC
- **Descrizione task**: Nota integrativa sulla canonizzazione del Compose di root per registrare l'allineamento dei file di configurazione montati dai servizi.
- **File modificati**: `configs/node1.yaml`, `configs/node2.yaml`, `configs/node3.yaml`, `docs/operational_log.md`.
- **Reasoning summary**: Ho aggiornato i `seed_peers` dei tre file `configs/node*.yaml` ai nomi DNS effettivamente risolvibili nella rete Compose (`node1`, `node2`, `node3`), così che i peer dichiarati nei file montati dai container coincidano con gli hostname dei servizi definiti nel `docker-compose.yml` canonico.

## 2026-03-19 09:00:08 UTC
- **Descrizione task**: Introduzione del `Dockerfile` multi-stage per il nodo SDCC, migrazione del Compose a immagine applicativa locale e aggiornamento della documentazione di deployment.
- **File modificati**: `Dockerfile`, `docker-compose.yml`, `deploy/docker-compose.yml`, `README.md`, `docs/deployment.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho sostituito l'esecuzione `go run` in container `golang:1.22` con una build multi-stage che produce il binario `sdcc-node` e lo esegue in un runtime minimale distroless; in parallelo ho aggiornato il Compose canonico per usare `build:` e mount della sola configurazione, mantenendo l'avvio coerente con `--config /config/config.yaml` e riallineando la documentazione operativa al nuovo flusso di avvio locale.
## 2026-03-19 09:20:00 UTC
- **Descrizione task**: Riscrittura di `docs/deployment.md` come documento canonico del deployment locale multi-nodo con Docker Compose, con allineamento esplicito a README, file Compose di root e configurazioni `configs/node*.yaml`.
- **File modificati**: `docs/deployment.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho trasformato la documentazione di deployment da nota sintetica a guida canonica operativa, includendo prerequisiti, comandi standard, spiegazione della rete Compose e della risoluzione DNS tramite service name, oltre a una sezione troubleshooting focalizzata sui problemi realmente plausibili nel cluster locale attuale (bootstrap race condition, mismatch YAML/env, nomi DNS, differenze ambientali Docker e membership non convergente). Ho anche riallineato il README per chiarire che `docs/deployment.md` è il riferimento operativo canonico del deployment locale multi-nodo.
## 2026-03-19 09:19:23 UTC
- **Descrizione task**: Aggiornamento del README nelle sezioni Docker Compose e demo rapida per chiarire il file Compose canonico M07, i comandi reali di gestione del cluster e l’allineamento tra rete Compose e configurazioni `configs/node*.yaml`.
- **File modificati**: `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho reso esplicito che il file operativo canonico è `docker-compose.yml` alla root, ho chiarito il ruolo secondario/storico di `deploy/docker-compose.yml`, ho aggiunto i comandi reali `docker compose ...` inclusi i log di `node1` e ho documentato che discovery, porte e peer dei tre file `configs/node*.yaml` coincidono con la rete Compose e con gli override environment del runtime.

## 2026-03-19 09:25:48 UTC
- **Descrizione task**: Creazione del documento milestone `docs/task/M07.md` per Compose + networking, con stato iniziale, gap rispetto ai done criteria, modifiche introdotte e risultato operativo atteso del flusso Docker Compose.
- **File modificati**: `docs/task/M07.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho analizzato task M01-M06, README, `docker-compose.yml`, `docs/deployment.md`, roadmap e log operativo per mantenere lo stesso formato documentale; quindi ho consolidato M07 come milestone di deployment locale, esplicitando l'evoluzione dal compose placeholder/root al compose canonico con rete dedicata `sdcc-net` e discovery tramite service name DNS.

## 2026-03-19 10:15:00 UTC
- **Descrizione task**: Riallineamento del bootstrap membership per separare `node_id` logico e `addr` di rete, introduzione di `advertise_addr`, aggiornamento configurazioni Compose e copertura test su fallback seed/discovery con hostname Compose.
- **File modificati**: `cmd/node/main.go`, `internal/config/config.go`, `internal/config/config_test.go`, `internal/membership/bootstrap.go`, `internal/membership/bootstrap_test.go`, `internal/membership/membership.go`, `configs/node1.yaml`, `configs/node2.yaml`, `configs/node3.yaml`, `configs/example.yaml`, `docker-compose.yml`, `README.md`, `docs/deployment.md`, `docs/configuration.md`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho eliminato l'ambiguità precedente in cui il bootstrap inviava `Addr = node_id`, introducendo un endpoint pubblicizzato esplicito `advertise_addr` e usando sempre `host:port` come indirizzo di rete reale. Ho inoltre reso il bootstrap seed-only capace di riconciliare i placeholder iniziali con il vero `node_id` appreso via gossip/join, e ho riallineato file YAML e Compose ai service name DNS (`node1`, `node2`, `node3`) come hostname raggiungibili.

## 2026-03-19 10:28:00 UTC
- **Descrizione task**: Correzione emersa in validazione sulla semantica di merge `min`/`max`, per preservare l'applicazione degli update lenti per-contributo senza alterare le regole di conflitto globali di `sum`/`average`.
- **File modificati**: `internal/gossip/state.go`, `docs/operational_log.md`.
- **Reasoning summary**: Durante l'esecuzione della suite completa ho rilevato che i casi `nodo_lento` per `min`/`max` non venivano applicati correttamente a causa del filtro globale sulle versioni. Ho quindi limitato il merge per-contributo speciale ai soli casi `min`/`max`, mantenendo invariata la semantica storica dei conflitti per `sum` e `average` e ripristinando il passaggio dell'intera suite `go test ./...`.
## 2026-03-19 10:45:00 UTC
- **Descrizione task**: Creazione del documento milestone `docs/task/M08.md` e aggiornamento del `README.md` per dichiarare esplicitamente lo stato post-M08 della copertura test e il comando unico di verifica richiesto.
- **File modificati**: `docs/task/M08.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho analizzato README, architettura, configurazione, log operativo e milestone M05-M07 per mantenere il formato documentale esistente; quindi ho consolidato in M08 lo stato iniziale della copertura test per `merge`, `membership`, `config` e `aggregation`, i gap documentali rilevati, le attività di verifica eseguite e l'esito finale rispetto ai done criteria, aggiungendo nel README il comando repository-wide `go test ./... -run Test -count=1` come verifica post-milestone.
## 2026-03-19 11:40:00 UTC
- **Descrizione task**: Stabilizzazione temporale dei test M08 di membership/bootstrap/gossip sostituendo i riferimenti al clock reale con timestamp fissi e introducendo un helper deterministico per i tombstone `leave`.
- **File modificati**: `internal/membership/bootstrap_test.go`, `internal/membership/membership_test.go`, `internal/gossip/membership_convergence_test.go`, `internal/membership/membership.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho rimosso dai test target ogni dipendenza da `time.Now().UTC()` introducendo basi temporali locali con `time.Date(...)` e offset espliciti `base.Add(...)`; inoltre ho aggiunto `LeaveAt` nel package membership per evitare che `TestJoinLeave` richiamasse indirettamente il clock di processo tramite `Leave`, mantenendo invariata la API runtime esistente grazie al wrapper compatibile `Leave`.
## 2026-03-19 14:45:19 UTC
- **Descrizione task**: Estensione dei test di configurazione per congelare la semantica degli override environment malformati e chiarimento documentale del comportamento reale di `Load`.
- **File modificati**: `internal/config/config_test.go`, `README.md`, `docs/configuration.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho verificato che il comportamento reale di `overrideFromEnv` ignora gli override numerici/CSV malformati invece di far fallire il caricamento; ho quindi aggiunto subtest dedicati per `NODE_PORT`, `FANOUT`, `ENABLED_AGGREGATIONS` e `BOOTSTRAP_PEERS` con base/file validi, e ho reso esplicita la stessa semantica nella documentazione utente e canonica di configurazione.
## 2026-03-19 14:53:20 UTC
- **Descrizione task**: Estensione di internal/gossip/state_test.go con copertura esplicita dei rami applyRemote per mismatch aggregazione, stessa versione/payload e versione remota più vecchia non stale.
- **File modificati**: \, \.
- **Reasoning summary**: Ho aggiunto tre subtest mirati in \ per congelare i rami \, \ e \, verificando esito merge, reason, invarianza di \ e semantica di aggiornamento di \ e \ coerente con l'implementazione di \.
## 2026-03-19 14:53:45 UTC
- **Descrizione task**: Correzione integrativa del log operativo precedente per registrare in forma completa l'estensione dei test di merge gossip.
- **File modificati**: `internal/gossip/state_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho aggiunto tre subtest mirati in `TestMergeRules` per congelare i rami `aggregation_type_mismatch`, `same_version_same_payload` e `older_version`, verificando esito merge, reason, invarianza di `State.Value` e semantica di aggiornamento di `SeenMessageIDs` e `LastSeenVersionByNode` coerente con l'implementazione di `applyRemote`.
## 2026-03-19 15:03:09 UTC
- **Descrizione task**: Estensione dei test unitari per i merge base delle aggregazioni `sum`, `average`, `min` e `max`, con contratto numerico esplicito su input negativi, uguali, valori molto grandi, `±Inf` e `NaN`.
- **File modificati**: `internal/aggregation/aggregation_test.go`, `internal/aggregation/sum/algorithm.go`, `internal/aggregation/average/algorithm.go`, `internal/aggregation/min/algorithm.go`, `internal/aggregation/max/algorithm.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho mantenuto i test nel package radice `internal/aggregation` per esercitare la factory reale e documentare in un unico punto il contratto numerico delle primitive `Merge`. Ho reso esplicito nei commenti degli algoritmi che gli input finiti e `±Inf` sono supportati secondo IEEE-754, mentre `NaN` non è considerato input semantico supportato: per `sum`/`average` viene propagato dall'aritmetica Go, per `min`/`max` viene congelato il comportamento dei confronti float64 (NaN remoto ignorato, NaN locale preservato).
## 2026-03-19 16:05:00 UTC
- **Descrizione task**: Introduzione della milestone M09 con suite di integrazione canonica `tests/integration/TestClusterConvergence`, documento `docs/testing.md`, task file dedicato e aggiornamento README con comando operativo ufficiale.
- **File modificati**: `tests/integration/cluster_convergence_test.go`, `docs/testing.md`, `docs/task/M09.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho estratto la verifica di convergenza cluster in una suite di integrazione esplicita fuori dai package interni, usando una rete in-memory deterministicamente controllata per evitare dipendenze da UDP/Docker e congelando soglia (`0.05`), round (`10ms`) e timeout (`2s`) nella documentazione canonica dei test e nel README.

## 2026-03-19 15:46:30 UTC
- **Descrizione task**: Rafforzamento di `tests/integration/TestClusterConvergence` riusando il riferimento logico di `internal/gossip/integration_test.go` per polling/timeout, bootstrap automatico del cluster in-memory promosso e report finale leggibile per nodo.
- **File modificati**: `tests/integration/cluster_convergence_test.go`, `docs/testing.md`, `docs/task/M09.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho promosso esplicitamente la strategia di bootstrap a harness in-memory, mantenendo round da `10ms`, polling con ticker da `20ms` e timeout massimo di `2s` in linea con il riferimento logico già presente nei test di integrazione interni. Ho reso il criterio di convergenza esplicito come banda massima tra nodi (`max(values)-min(values) <= 0.05`), evitando sleep arbitrari grazie a `time.NewTicker` + timer di timeout, e ho aggiunto un report finale via `t.Logf` che mostra valori per nodo, media iniziale di riferimento, banda cluster e offset massimo dal riferimento.
## 2026-03-19 15:51:24 UTC
- **Descrizione task**: Riallineamento del target `test-integration` al comando canonico M09 e chiarimento documentale della distinzione tra test interni in-memory, suite di integrazione end-to-end M09 e scenario operativo con cluster locale multi-nodo su Docker Compose.
- **File modificati**: `Makefile`, `README.md`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho aggiornato il `Makefile` affinché `test-integration` esegua la suite canonica `tests/integration/TestClusterConvergence`, aggiungendo `test-integration-internal` per conservare il vecchio controllo in-memory del package `internal/gossip` senza ambiguità rispetto a M09. In parallelo ho rivisto README e documento canonico dei test per rendere coerente la terminologia: i test interni in-memory restano separati dalla suite di integrazione end-to-end M09, mentre Docker Compose viene descritto come scenario distinto di cluster locale multi-nodo per validazioni operative/manuali.

## 2026-03-19 16:05:00 UTC
- **Descrizione task**: Consolidamento documentale e tecnico dello scenario M09 con timeout esplicito motivato, parametri centralizzati nel test di integrazione e formato report finale per nodo.
- **File modificati**: `tests/integration/cluster_convergence_test.go`, `docs/testing.md`, `docs/task/M09.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho reso i parametri M09 facilmente rintracciabili centralizzandoli in costanti, ho sostituito il timeout letterale con una composizione esplicita coerente con `gossip_interval` e con la variabilità locale/CI, e ho allineato documentazione e report finale del test al formato richiesto per facilitare diagnosi e manutenzione.

## 2026-03-19 16:35:00 UTC
- **Descrizione task**: Introduzione di helper script idempotenti per bootstrap/attesa/raccolta artefatti/teardown del cluster Docker Compose, con osservabilità dei valori finali in shutdown e documentazione operativa aggiornata.
- **File modificati**: `scripts/cluster_common.sh`, `scripts/cluster_up.sh`, `scripts/cluster_wait_ready.sh`, `scripts/cluster_collect_results.sh`, `scripts/cluster_down.sh`, `cmd/node/main.go`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho aggiunto un set minimo di script Bash per orchestrare esternamente il Compose canonico senza modificarne la struttura, con cleanup preventivo, naming stabile (`sdcc-bootstrap`) e messaggi di errore uniformi. Per permettere la raccolta dei valori finali per nodo senza introdurre endpoint runtime aggiuntivi, ho reso esplicito nel binario il log di shutdown con snapshot finale (`final_value`, round e message id), quindi ho documentato in `docs/testing.md` il flusso operativo consigliato e la directory artefatti generata dagli helper.

## 2026-03-20 00:00:00 UTC
- **Descrizione task**: Introduzione del test canonico `TestNodeCrashAndRestart` nella suite `tests/integration`, con estrazione dell'harness in-memory condiviso, logging osservabile e aggiornamento documentazione dei test.
- **File modificati**: `tests/integration/harness_test.go`, `tests/integration/cluster_convergence_test.go`, `tests/integration/node_crash_restart_test.go`, `docs/testing.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho centralizzato rete/transport/bootstrap/polling/snapshot in helper condivisi per evitare duplicazioni nella suite di integrazione; il nuovo scenario verifica attività gossip pre-crash, stop effettivo del nodo, convergenza del cluster residuo, restart/rejoin e aggiornamento del nodo rientrato tramite assert osservabili e `t.Logf` diagnostici, mantenendo la documentazione allineata al comportamento reale del repository.

## 2026-03-20 00:10:00 UTC
- **Descrizione task**: Correzione concorrente del payload gossip per la suite di integrazione tramite copia profonda dello stato serializzabile prima della `json.Marshal`.
- **File modificati**: `internal/gossip/engine.go`, `docs/operational_log.md`.
- **Reasoning summary**: Durante l'esecuzione ripetuta dei test di integrazione è emersa una corsa su mappe condivise tra round gossip e merge in ricezione; ho quindi isolato il payload serializzato con una copia profonda dei metadati di aggregazione, così da rendere stabile il nuovo scenario crash/restart e la suite `tests/integration` senza alterare il contratto osservabile del protocollo.

## 2026-03-20 11:06:00 UTC
- **Descrizione task**: Rafforzamento dello scenario canonico crash/restart nella suite `tests/integration` con verifiche separate su cluster residuo, rejoin reale del nodo e stabilizzazione finale multi-snapshot.
- **File modificati**: `tests/integration/node_crash_restart_test.go`, `docs/testing.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho preso come riferimento i test interni `internal/gossip/integration_test.go` e ho irrigidito la suite canonica `tests/integration` aggiungendo assert su deregistrazione del nodo crashato dal transport di test, raccolta di snapshot consecutivi del cluster residuo per distinguere progresso/stabilizzazione coerente, verifica che il nodo riavviato non resti sul valore di restart e finestra finale di convergenza stabile su più poll consecutivi. Ho inoltre documentato perché `average` resta utile come aggregazione osservabile per il riallineamento del nodo rientrato e ho chiarito che il riferimento informativo finale viene derivato dal cluster residuo stabilizzato.

## 2026-03-20 12:10:00 UTC
- **Descrizione task**: Introduzione della directory `scripts/fault_injection/` con helper minimi per stop/start manuale di un nodo Compose e raccolta di snapshot diagnostici, più aggiornamento documentale sulla distinzione tra supporti operativi manuali e test automatici in-memory.
- **File modificati**: `scripts/fault_injection/common.sh`, `scripts/fault_injection/node_stop_start.sh`, `scripts/fault_injection/collect_debug_snapshot.sh`, `README.md`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho aggiunto helper Bash piccoli e riusabili che estendono `scripts/cluster_common.sh` senza introdurre orchestrazione centralizzata: uno script simula `stop`/`start`/`bounce` di un singolo servizio del Compose canonico e uno raccoglie snapshot diagnostici minimali in `artifacts/fault_injection/`. In parallelo ho documentato esplicitamente che il test automatico canonico di crash/restart resta nella suite `tests/integration` con harness in-memory, mentre i nuovi script sono pensati solo per validazione manuale e debug operativo del cluster Docker Compose locale.
## 2026-03-20 11:17:13 UTC
- **Descrizione task**: Estensione di `docs/testing.md` con sezione M10 separata da M09, chiarimento del rapporto tra test interni crash/rejoin, test canonico in `tests/integration` e script manuali di fault injection; allineamento sintetico del README alla nuova distinzione documentale.
- **File modificati**: `docs/testing.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho separato esplicitamente M09 (convergenza cluster) da M10 (crash/restart e rejoin) nel documento canonico dei test, documentando scenario, timeout motivati, polling/stabilizzazione, limiti noti e comando operativo ufficiale del test `TestNodeCrashAndRestart`; ho inoltre aggiornato il README per evitare divergenze terminologiche tra documentazione introduttiva e documento canonico.
## 2026-03-20 11:23:14 UTC
- **Task**: allineamento documentazione M10 nel README con separazione esplicita da M09 e aggiunta del task report dedicato.
- **File modificati**: `README.md`, `docs/task/M10.md`, `docs/operational_log.md`.
- **Sintesi ragionata**: ho verificato che il test canonico `TestNodeCrashAndRestart` e la documentazione `docs/testing.md` erano già presenti, ma mancava il task report dedicato richiesto per considerare M10 pienamente completata. Ho quindi aggiornato il README per distinguere chiaramente M09 da M10, correggere il timeout M09 a `350ms`, documentare il comando ufficiale M10 e aggiungere il report `docs/task/M10.md` per mantenere coerenza tra codice, documentazione e milestone.

## 2026-03-20 11:29:54 UTC
- **Descrizione task**: Riscrittura di `docs/task/M10.md` nello stile milestone M08/M09, con esplicitazione del percorso da copertura interna parziale a completamento nella suite canonica di integrazione.
- **File modificati**: `docs/task/M10.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho riletto README, documentazione architetturale/testing, log operativo, baseline interna `internal/gossip/integration_test.go`, harness `tests/integration` e script `scripts/` per ricostruire lo stato reale di M10. Ho quindi riscritto il task report chiarendo che la milestone era assente come documento, parzialmente coperta da test interni e poi completata nella suite canonica `tests/integration/TestNodeCrashAndRestart`, includendo gap, componenti riusati, decisioni tecniche, strategia di test, verifiche, limiti residui e follow-up.

## 2026-03-20 12:30:00 UTC
- **Descrizione task**: Introduzione di un target Makefile dedicato al test canonico M10 e chiarimento documentale della differenza tra target crash interno/debug e target crash/restart di milestone.
- **File modificati**: `Makefile`, `README.md`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho mantenuto invariato `make test-crash` come entry point dei test interni `internal/gossip`, aggiungendo `make test-crash-restart` e l'alias `make test-m10` verso `tests/integration/TestNodeCrashAndRestart`. Ho poi allineato README e documento canonico dei test per esplicitare senza ambiguità la differenza tra livello interno/debug e livello canonico M10.
## 2026-03-20 11:48:36 UTC
- **Descrizione task**: Migrazione della suite di test dai package sotto `internal/` alla nuova gerarchia speculare `tests/`, con riallineamento dei comandi documentati, dei target Makefile e delle API minime necessarie per mantenere i test esterni coerenti.
- **File modificati**: `Makefile`, `README.md`, `docs/testing.md`, `docs/task/M01.md`, `docs/task/M02.md`, `docs/task/M03.md`, `docs/task/M04.md`, `docs/task/M05.md`, `docs/task/M06.md`, `docs/task/M08.md`, `docs/task/M10.md`, `docs/operational_log.md`, `internal/gossip/engine.go`, `internal/gossip/state.go`, `tests/config/config_test.go`, `tests/config/reexport.go`, `tests/membership/bootstrap_test.go`, `tests/membership/membership_test.go`, `tests/membership/reexport.go`, `tests/gossip/engine_test.go`, `tests/gossip/engine_transport_contract_test.go`, `tests/gossip/integration_test.go`, `tests/gossip/membership_convergence_test.go`, `tests/gossip/reexport.go`, `tests/gossip/state_test.go`, `tests/gossip/sum_regression_test.go`, `tests/transport/contract_test.go`, `tests/transport/reexport.go`, `tests/transport/transport_test.go`, `tests/transport/udp_transport_test.go`, `tests/observability/logger_test.go`, `tests/observability/reexport.go`, `tests/aggregation/aggregation_test.go`, `tests/aggregation/reexport.go`, `tests/aggregation/average/average_convergence_test.go`, `tests/aggregation/max/max_convergence_test.go`, `tests/aggregation/min/min_convergence_test.go`, `tests/aggregation/sum/sum_convergence_test.go`.
- **Reasoning summary**: Ho prima ricostruito la mappa completa dei test e dei riferimenti documentali/operativi, poi ho spostato tutti i file `*_test.go` da `internal/` a `tests/` mantenendo una struttura speculare per modulo. Per limitare i cambiamenti sulla logica applicativa ho introdotto piccoli shim di re-export nei package di test e ho esposto in `internal/gossip` solo i punti minimi necessari a preservare i contratti già verificati dalla suite esterna (`ApplyRemote`, `NormalizeStateVersion`, `MergeMembership`, `RoundOnce`, `CurrentMessageVersion`). Infine ho riallineato Makefile, README e documentazione dei task ai nuovi path e ho validato sia `go test ./tests/... -count=1` sia la suite repository-wide `go test ./... -count=1`.
## 2026-03-20 13:56:14 UTC
- **Descrizione task**: Implementazione API minima di observability con logger `slog`, collector metriche aggregate a bassa cardinalità, endpoint HTTP `/health` `/ready` `/metrics`, test canonico `TestMetricsExposure` e aggiornamento documentazione.
- **File modificati**: `internal/observability/logger.go`, `internal/observability/metrics.go`, `internal/observability/metrics_test.go`, `README.md`, `docs/architecture.md`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho mantenuto l’API indipendente dal runtime corrente per evitare introdurre configurazione non richiesta, scegliendo un formato testuale stabile per `/metrics` e riducendo la cardinalità dei label al solo esito di merge (`applied`, `skipped`, `conflict`, `unknown`); il test canonico valida esposizione minima, health positivo e readiness coerente con lo stato del collector.


## 2026-03-20 13:20:00 UTC
- **Descrizione task**: Uniformazione del logging strutturato degli eventi gossip in `cmd/node/main.go` e `internal/gossip/engine.go`, con chiavi stabili per bootstrap, avvio transport, round, merge remoto e shutdown.
- **File modificati**: `cmd/node/main.go`, `internal/gossip/engine.go`, `tests/gossip/engine_test.go`, `scripts/cluster_wait_ready.sh`, `docs/testing.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho introdotto un formato coerente per i principali eventi di debugging del gossip usando chiavi stabili (`event`, `node_id`, `round`, `peers`, `estimate`) e ho separato il dettaglio più verboso del merge remoto mantenendo a livello `info` solo i casi applicati o conflittuali. Ho inoltre aggiornato i controlli degli script/documentazione che dipendevano dai vecchi messaggi testuali e ho aggiunto test dedicati per congelare sia la presenza dei campi strutturati nei round sia il fatto che i merge remoti non serializzino payload completi o metadati troppo rumorosi nei log.
## 2026-03-20 14:40:05 UTC
- **Descrizione task**: Integrazione del lifecycle reale di `cmd/node/main.go` con lo strato di observability, aggiungendo stato minimo del nodo e semantica più utile per `/health` e `/ready`.
- **File modificati**: `cmd/node/main.go`, `internal/observability/metrics.go`, `internal/observability/metrics_test.go`, `README.md`, `docs/architecture.md`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho evitato di introdurre un nuovo package `internal/node/` perché il wiring richiesto resta piccolo e coerente con il layout attuale, usando quindi `cmd/node/main.go` come punto di integrazione primario. Ho aggiunto nel collector uno stato lifecycle monotono (`startup`, `bootstrap_completed`, `transport_initialized`, `engine_started`, `shutdown`), ho collegato `/health` a una liveness di processo sempre positiva con `node_state` utile al debug e ho reso `/ready` dipendente dall'avvenuto bootstrap e dallo start effettivo dell'engine. Ho inoltre avviato il server HTTP di observability direttamente dal runtime del nodo con override opzionale `OBSERVABILITY_ADDR`, mantenendo la modifica piccola e allineando test e documentazione alla semantica reale.

## 2026-03-20 15:10:00 UTC
- **Descrizione task**: Chiusura documentale M11 con nuovo documento canonico di observability, task report dedicato e aggiornamento README sullo stato post-milestone e sul comando di verifica ufficiale.
- **File modificati**: `docs/observability.md`, `docs/task/M11.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho prima ricostruito lo stato reale dell'observability già presente nel repository tramite README, architettura, testing e log operativo. Ho quindi consolidato in `docs/observability.md` l'architettura minima, i campi log, le metriche, gli endpoint, le istruzioni d'uso e i limiti noti senza introdurre claim oltre l'implementazione attuale; in parallelo ho creato `docs/task/M11.md` nello stile delle milestone M08-M10 e aggiornato il README per dichiarare esplicitamente il comando canonico `go test ./internal/observability -run TestMetricsExposure`.

## 2026-03-20 14:59:01 UTC
- **Descrizione task**: Chiarimento documentale definitivo della scelta observability come soluzione ibrida, con esplicitazione di porta HTTP, lifecycle del server e criteri readiness per evitare implementazioni duplicate nei task successivi.
- **File modificati**: `docs/observability.md`, `README.md`, `docs/task/M11.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho ricostruito la baseline reale del repository e verificato che l'implementazione corrente combina già logger strutturato e endpoint HTTP minimi. Ho quindi reso la decisione architetturale esplicita e vincolante nella documentazione canonica, separando chiaramente le responsabilità di stdout e HTTP e fissando senza ambiguità porta di default, lifecycle del server e semantica di readiness.

## 2026-03-23 23:20:00 UTC
- **Descrizione task**: Implementazione del client reale di join iniziale via HTTP, integrazione nel runtime del nodo con fallback ai peer di discovery e aggiunta di un test di integrazione mirato sul bootstrap con `join_endpoint`.
- **File modificati**: `cmd/node/main.go`, `internal/membership/join_http_client.go`, `tests/integration/join_endpoint_bootstrap_test.go`, `README.md`, `docs/architecture.md`, `docs/testing.md`, `docs/task/M06.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho prima ricostruito il flusso di bootstrap esistente e verificato che `membership.Bootstrap` gestisse già correttamente il fallback su `DiscoveryPeers()`, ma che il runtime usasse ancora `NoopJoinClient`. Ho quindi introdotto un adapter HTTP concreto che invia `JoinRequest` a `http://<join_endpoint>/join`, decodifica `JoinResponse` con `snapshot` e `delta` e viene selezionato automaticamente da `cmd/node/main.go` solo quando `join_endpoint` è configurato. Per validare il comportamento end-to-end ho aggiunto un test di integrazione che avvia realmente il binario del nodo, espone un join endpoint HTTP con `httptest` e verifica che il nodo popoli la membership iniziale osservando sia la richiesta HTTP di join sia un payload gossip UDP verso il peer restituito dal bootstrap.

## 2026-03-23 18:42:15 UTC
- **Descrizione task**: Collegamento della configurazione runtime `membership_timeout_ms` ai timeout effettivi della membership, con mappatura stabile documentata e test osservabili sulle transizioni `alive -> suspect -> dead`.
- **File modificati**: `cmd/node/main.go`, `internal/config/config.go`, `tests/membership/runtime_timeout_mapping_test.go`, `docs/configuration.md`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho sostituito il wiring del nodo per usare `membership.NewSetWithConfig(cfg.MembershipConfig())`, introducendo una traduzione esplicita del singolo parametro runtime in due timeout interni coerenti (`SuspectTimeout = max(1ms, timeout/2)` e `DeadTimeout = max(SuspectTimeout + 1ms, timeout)`). Ho congelato la regola con test che mostrano differenze osservabili tra timeout corti e lunghi e con un edge case sui valori minimi, poi ho aggiornato la documentazione canonica per evitare divergenze tra configurazione, architettura e comportamento reale.

## 2026-03-23 18:50:23 UTC
- **Descrizione task**: Integrazione della failure detection nel loop runtime gossip con heartbeat implicito del mittente, logging strutturato delle transizioni membership e nuovo test di integrazione sul degrado automatico `alive -> suspect -> dead`.
- **File modificati**: `internal/gossip/engine.go`, `internal/membership/membership.go`, `tests/integration/runtime_membership_failure_detection_test.go`, `docs/architecture.md`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho scelto `internal/gossip/engine.go` come punto di integrazione per mantenere la failure detection accoppiata al tick reale del runtime invece che al solo wiring di `cmd/node`. A ogni round l'engine applica i timeout membership prima di selezionare i target, tratta ogni messaggio gossip valido come heartbeat implicito del `origin_node` e logga le transizioni di stato con evento stabile `membership_transition`. Ho poi aggiunto un test di integrazione che ferma un nodo del cluster in-memory e verifica che un peer superstite osservi automaticamente il degrado `suspect` e poi `dead` senza interventi diretti del test sulla membership.

## 2026-03-23 19:03:07 UTC
- **Descrizione task**: Chiarimento della semantica di rimozione membership con retention temporanea dei tombstone `dead`/`leave`, prune deterministica e protezione contro la reintroduzione di digest gossip obsoleti.
- **File modificati**: `internal/membership/membership.go`, `internal/gossip/engine.go`, `tests/membership/membership_test.go`, `tests/gossip/membership_convergence_test.go`, `docs/architecture.md`, `docs/task/M02.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho scelto di non mantenere `dead`/`leave` per sempre nella membership attiva: i peer restano come tombstone solo per una retention separata (`PruneRetention`), poi vengono rimossi fisicamente tramite `Prune(now)` durante il loop gossip. Per evitare che una prune anticipata reintroduca peer obsoleti, il set conserva un watermark locale minimale del peer potato e rifiuta update con `incarnation` non strettamente più fresca; un rejoin resta quindi possibile solo con `incarnation` superiore. Ho aggiornato la documentazione canonica e aggiunto test sia sul package membership sia sul merge gossip per congelare retention, prune e blocco dei digest obsoleti.

## 2026-03-23 19:12:30 UTC
- **Descrizione task**: Consolidamento M02 con test dedicati ai casi rischiosi di membership gossip e aggiornamento della documentazione canonica dei comandi di verifica.
- **File modificati**: `tests/gossip/membership_convergence_test.go`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho aggiunto quattro scenari mirati per congelare i rischi dichiarati di M02: riconvergenza dopo partizione temporanea modellata come viste membership divergenti tra sottoinsiemi di nodi, recupero di un peer `suspect` tramite update `alive` con `incarnation` maggiore, rejoin obsoleto con `incarnation` minore ignorato e riallineamento tra placeholder seed `host:port` e `node_id` canonico. Ho inoltre aggiornato `docs/testing.md` con la sezione canonica dei comandi M02 per mantenere allineati test automatici e documentazione operativa.

## 2026-03-23 19:25:50 UTC
- **Descrizione task**: Riallineamento di roadmap, task M02, README e guida ai test agli entrypoint reali della suite esterna per la membership.
- **File modificati**: `ROADMAP.md`, `README.md`, `docs/testing.md`, `docs/task/M02.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho verificato la struttura reale dei test sotto `tests/` e ho rimosso i riferimenti fuorvianti a package interni senza file `*_test.go`, distinguendo esplicitamente tre livelli di verifica per M02: suite unitaria membership, suite gossip membership e test di integrazione runtime. Ho inoltre uniformato i comandi canonici con `-count=1` per evitare ambiguità rispetto alla cache dei test e mantenere la documentazione coerente con il layout corrente del repository.

## 2026-03-23 19:49:59 UTC
- **Descrizione task**: Allineamento dei riferimenti documentali M03 al package reale della suite `TestTransportContract`.
- **File modificati**: `ROADMAP.md`, `docs/task/M03.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho verificato tutti i riferimenti testuali a `TestTransportContract` nel repository e ho corretto le occorrenze documentali che puntavano ancora a `./internal/...`, uniformando il comando canonico M03 a `go test ./tests/transport -run TestTransportContract` per riflettere il package reale dei test senza introdurre divergenze tra roadmap e task milestone.

## 2026-03-23 19:55:01 UTC
- **Descrizione task**: Revisione documentale della milestone M03 per allineare i riferimenti delle cartelle coinvolte al punto di integrazione runtime reale del repository.
- **File modificati**: `ROADMAP.md`, `docs/task/M03.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho verificato che il wiring reale della milestone M03 passa da `cmd/node/main.go` e che il package `internal/node/` non esiste nel layout corrente. Ho quindi sostituito in roadmap il riferimento fuorviante con `cmd/node/` e ho chiarito nel task M03 che `internal/node/` era solo un riferimento architetturale storico non più usato, così da evitare ambiguità per verificatori e implementatori futuri.


## 2026-03-23 20:01:37 UTC
- **Descrizione task**: Spostamento del test canonico delle metriche observability da `internal/observability/` a `tests/observability/`, con riallineamento dei riferimenti documentali e dei comandi di verifica.
- **File modificati**: `README.md`, `ROADMAP.md`, `docs/observability.md`, `docs/testing.md`, `docs/task/M11.md`, `docs/operational_log.md`, `tests/observability/metrics_test.go`.
- **Reasoning summary**: Ho verificato le convenzioni già adottate sotto `tests/observability/`, dove i test esterni usano il package logico `observability` insieme a un piccolo shim di re-export (`tests/observability/reexport.go`). Per mantenere compatibilità con il layout corrente ho quindi spostato `TestMetricsExposure` in `tests/observability/metrics_test.go` senza dipendenze da simboli non esportati, mantenendo package e import coerenti con il package logico. Infine ho aggiornato la documentazione che assumeva ancora il vecchio path interno, così da mantenere coerenti layout dei test, comandi canonici e guida observability.

## 2026-03-23 20:12:00 UTC
- **Descrizione task**: Riallineamento documentale della milestone M04 al path reale della suite `TestSumConvergence` e chiarimento dei confini tra algoritmo base `sum`, merge gossip idempotente e test canonico di convergenza.
- **File modificati**: `ROADMAP.md`, `README.md`, `docs/task/M04.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho verificato prima il layout reale del repository, la documentazione canonica, il log operativo, l'implementazione dell'algoritmo base in `internal/aggregation/sum/`, il merge gossip in `internal/gossip/state.go` e la suite esterna `tests/aggregation/sum/sum_convergence_test.go`. Ho quindi corretto i riferimenti testuali ancora fuorvianti verso `./internal/aggregation/sum`, uniformando il comando canonico a `go test ./tests/aggregation/sum -run TestSumConvergence` e chiarendo nella milestone M04 dove vive l'algoritmo base, dove si implementa la convergenza distribuita/idempotente e quale sia la suite canonica di verifica.

## 2026-03-23 20:25:03 UTC
- **Descrizione task**: Riallineamento documentale della milestone M05 ai path reali delle suite di convergenza `average`/`min`/`max` e della regressione multi-aggregazione, con standardizzazione dei comandi in README, roadmap e task milestone.
- **File modificati**: `ROADMAP.md`, `README.md`, `docs/task/M05.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho verificato il layout reale delle suite sotto `tests/aggregation/*` e `tests/gossip`, confermando che il riferimento storico a `./internal/aggregation/...` non era più coerente con la convenzione adottata nel repository. Ho quindi standardizzato i comandi M05 sui package canonici esterni (`average`, `min`, `max`) mantenendo separata la regressione `sum` nel package `tests/gossip`, senza introdurre wrapper o spostamenti di test perché la convenzione repository-wide è già stabilmente basata su `tests/`.

## 2026-03-23 20:54:02 UTC
- **Descrizione task**: Rafforzamento del load config M06 per rendere fail-fast gli override environment numerici/CSV malformati, con aggiornamento test di regressione e documentazione coerente.
- **File modificati**: `internal/config/config.go`, `tests/config/config_test.go`, `docs/configuration.md`, `README.md`, `docs/task/M06.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho mantenuto la precedence `Default() -> file -> env -> Validate()` ma ho trasformato gli helper di override env numerici/CSV in controlli espliciti che falliscono subito quando una variabile presente contiene valori non parseabili o CSV con item vuoti. Ho poi aggiornato la suite M06 per congelare i casi `NODE_PORT=abc`, `FANOUT=abc`, `ENABLED_AGGREGATIONS=sum,,max` e `BOOTSTRAP_PEERS=node-1:7001,` come errori attesi, e ho riallineato la documentazione utente/canonica alla nuova semantica fail-fast.

## 2026-03-23 21:05:48 UTC
- **Descrizione task**: Introduzione di un artefatto ripetibile di verifica per M07 basato sul flusso Compose canonico, con raccolta minima di  e log dei tre nodi, più aggiornamento dei criteri osservabili in documentazione.
- **File modificati**: , , , , .
- **Reasoning summary**: Ho prima ricostruito il flusso Compose canonico e i marker di log già presenti nel runtime (, , , ) per evitare di introdurre nuove superfici o claim non supportati. Ho quindi aggiunto uno script dedicato che esegue dalla root , attende i primi round e salva in  lo stato dei servizi e una quota minima di log per , , ; infine ho documentato in modo esplicito quali evidenze nei log dimostrano bootstrap completato, discovery tramite service name Compose e membership iniziale non vuota o convergente. La validazione strutturale dello script è stata eseguita con , mentre la verifica runtime Docker non è risultata eseguibile nell'ambiente corrente perché il comando  non è disponibile nel PATH.


## 2026-03-23 21:05:59 UTC
- **Descrizione task**: Introduzione di un artefatto ripetibile di verifica per M07 basato sul flusso Compose canonico, con raccolta minima di `docker compose ps` e log dei tre nodi, più aggiornamento dei criteri osservabili in documentazione.
- **File modificati**: `.gitignore`, `scripts/m07_collect_compose_evidence.sh`, `docs/deployment.md`, `docs/task/M07.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho prima ricostruito il flusso Compose canonico e i marker di log già presenti nel runtime (`gossip bootstrap completato`, `transport gossip avviato`, `remote_merge`, `gossip_round`) per evitare di introdurre nuove superfici o claim non supportati. Ho quindi aggiunto uno script dedicato che esegue dalla root `docker compose up -d --build`, attende i primi round e salva in `artifacts/m07/` lo stato dei servizi e una quota minima di log per `node1`, `node2`, `node3`; infine ho documentato in modo esplicito quali evidenze nei log dimostrano bootstrap completato, discovery tramite service name Compose e membership iniziale non vuota o convergente. La validazione strutturale dello script è stata eseguita con `bash -n`, mentre la verifica runtime Docker non è risultata eseguibile nell'ambiente corrente perché il comando `docker` non è disponibile nel PATH.


## 2026-03-23 21:06:08 UTC
- **Descrizione task**: Chiarimento del log operativo M07 per invalidare l'entry tecnica parziale generata durante l'append automatica precedente.
- **File modificati**: `docs/operational_log.md`.
- **Reasoning summary**: Durante il primo append del log operativo una here-doc non quotata ha prodotto un'entry parziale con backtick espansi dalla shell. In conformità con la regola di append-only non ho alterato la riga precedente, ma aggiungo questa nota esplicita: l'entry `2026-03-23 21:05:48 UTC` è da considerare non valida e va ignorata; la descrizione corretta dell'attività M07 è quella registrata nell'entry `2026-03-23 21:05:59 UTC`.

## 2026-03-23 21:11:54 UTC
- **Descrizione task**: Riallineamento delle sezioni iniziali di `docs/deployment.md` alla sorgente di verità runtime attuale del deployment Compose canonico.
- **File modificati**: `docs/deployment.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho verificato che il `docker-compose.yml` di root non definisce override via `environment:` e che la configurazione runtime dei tre nodi deriva esclusivamente dai file montati `configs/node1.yaml`, `configs/node2.yaml` e `configs/node3.yaml`. Ho quindi corretto nelle sezioni iniziali e nei punti di troubleshooting di `docs/deployment.md` i riferimenti che descrivevano `node_port` o i peer come duplicati dagli override ambiente del Compose, sostituendoli con una descrizione coerente con l'attuale sorgente di verità documentata nel repository.

## 2026-03-23 21:32:24 UTC
- **Descrizione task**: Promozione del test canonico M09 a scenario Compose reale con cluster locale multi-nodo, mantenendo una variante veloce in-memory e allineando configurazione/documentazione ai parametri effettivi di convergenza.
- **File modificati**: `tests/integration/compose_harness_test.go`, `tests/integration/cluster_convergence_test.go`, `internal/config/config.go`, `cmd/node/main.go`, `tests/config/config_test.go`, `configs/node1.yaml`, `configs/node2.yaml`, `configs/node3.yaml`, `configs/example.yaml`, `docs/testing.md`, `docs/task/M09.md`, `docs/configuration.md`, `README.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho prima ricostruito il flusso canonico Compose e il formato dei log/artefatti già esistenti per evitare superfici nuove non necessarie. Ho quindi introdotto un harness di integrazione reale che orchestra gli script `cluster_up`, `cluster_wait_ready` e `cluster_down`, raccoglie i valori finali dai log strutturati di shutdown e li riformatta con lo stesso schema M09. Per rendere il cluster Compose coerente con lo scenario documentato ho aggiunto `initial_value` alla configurazione runtime, impostando i tre nodi Compose su `average` con valori iniziali `10/30/50`, e ho aggiornato la documentazione per separare chiaramente suite canonica reale e suite veloce in-memory.

- **Data**: 2026-03-23
- **Ora**: 21:52 UTC
- **Descrizione task**: Introduzione della suite M10 reale Compose con fault injection automatica stop/start, mantenendo la variante rapida in-memory e riallineando documentazione, target Makefile e report milestone.
- **File modificati**: `tests/integration/compose_harness_test.go`, `tests/integration/node_crash_restart_compose_test.go`, `tests/integration/node_crash_restart_test.go`, `docs/testing.md`, `README.md`, `Makefile`, `docs/task/M10.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho prima ricostruito il flusso canonico dei test e degli script Compose/fault injection per rispettare il requisito di usare il cluster reale locale senza toccare l’harness in-memory esistente. Ho poi esteso l’harness Compose per leggere metriche live, gestire stop/start reali e raccogliere snapshot diagnostici; in parallelo ho promosso M10 a suite automatica reale (`TestNodeCrashAndRestart`) lasciando disponibile una variante veloce (`TestNodeCrashAndRestartInMemory`) per debugging locale. Infine ho aggiornato README, documento canonico dei test, target Makefile e task report M10 per rendere esplicita la distinzione tra test rapido in-memory e test lento/reale Compose, mantenendo coerenza documentale e operativa.

- **Data**: 2026-03-23
- **Ora**: 22:11 UTC
- **Descrizione task**: Integrazione del collector observability nel ciclo di vita runtime dell'engine gossip e del nodo, con aggiornamento delle metriche live durante round e merge remoti.
- **File modificati**: `internal/gossip/engine.go`, `cmd/node/main.go`, `tests/gossip/engine_test.go`, `tests/gossip/integration_test.go`, `tests/gossip/engine_transport_contract_test.go`, `tests/gossip/membership_convergence_test.go`, `tests/aggregation/average/average_convergence_test.go`, `tests/aggregation/max/max_convergence_test.go`, `tests/aggregation/min/min_convergence_test.go`, `tests/aggregation/sum/sum_convergence_test.go`, `tests/integration/harness_test.go`, `README.md`, `docs/architecture.md`, `docs/observability.md`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho analizzato prima documentazione canonica, architettura e log operativi, poi ho esteso `gossip.NewEngine` per accettare un collector opzionale condiviso con `cmd/node`. L'engine ora aggiorna il collector subito dopo round locali completati e dopo merge remoti, riallineando contatori e gauge allo stato runtime effettivo della membership e della stima locale. Ho aggiornato le suite che costruiscono l'engine, aggiunto test mirati per verificare incremento round/merge nel collector e riallineato la documentazione observability/testing per chiarire che `/metrics` deve riflettere progresso live e non soltanto snapshot di bootstrap/shutdown.

- Data: 2026-03-23
  Ora: 22:24:49 UTC
  Attività: Aggiornata la milestone M06 in ROADMAP.md per allineare il comando di verifica alla suite reale tests/config e chiarire che l'entrypoint canonico è tests/config/config_test.go.
  File modificati: ROADMAP.md
  Sintesi ragionamento: La roadmap riportava un comando puntato al package interno internal/config; l'allineamento documentale richiede invece di riferirsi alla suite reale nel package tests/config, coerente con TestValidateConfig già definito nel repository.

## 2026-03-23 22:30:00 UTC
- **Descrizione task**: Aggiornamento della milestone M08 in ROADMAP.md per allineare i riferimenti della suite di test alle directory reali centralizzate sotto tests/.
- **File modificati**: `ROADMAP.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho sostituito il riferimento legacy `internal/**/**/*_test.go` con le directory effettive `tests/gossip`, `tests/membership`, `tests/config`, `tests/aggregation`, `tests/transport` e `tests/observability`, chiarendo anche che il repository adotta una strategia di test esterna ai package interni e centralizzata sotto `SDCC-project/tests`.

## 2026-03-23 22:41:00 UTC
- **Descrizione task**: Aggiornamento della milestone M11 in `ROADMAP.md` per riallineare i riferimenti observability al bootstrap runtime reale del nodo e rimuovere il package legacy inesistente.
- **File modificati**: `ROADMAP.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho verificato prima la documentazione architetturale e il wiring reale in `cmd/node/main.go`, confermando che l'integrazione runtime dell'observability avviene nel bootstrap del nodo tramite collector, metrics handler e transizioni di lifecycle condivise con `internal/gossip/engine.go`. Ho quindi corretto la milestone M11 sostituendo il riferimento inesistente a `internal/node/` con `cmd/node/main.go` e aggiornando il testo dei criteri di completamento per descrivere il punto di integrazione runtime effettivo già documentato in `docs/architecture.md`.

## 2026-03-23 22:35:37 UTC
- **Descrizione task**: Aggiornata la sezione M07 in `ROADMAP.md` per sostituire il riferimento generico alla cartella Compose con i path reali presenti nel repository.
- **File modificati**: `ROADMAP.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho verificato prima la documentazione di deployment, il task M07 e gli artefatti reali di repository; quindi ho riallineato il campo "File/cartelle coinvolti" della roadmap ai file effettivamente presenti e pertinenti per la milestone Compose/networking, mantenendo `docs/deployment.md` come riferimento documentale canonico.

## 2026-03-23 22:43:56 UTC
- **Descrizione task**: Aggiornata `docs/deployment.md` per riallineare la sezione sulle aggregazioni dei tre servizi Compose alla configurazione runtime reale del cluster M09.
- **File modificati**: `docs/deployment.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho verificato prima la documentazione canonica di deployment/testing, le configurazioni `configs/node1.yaml`, `configs/node2.yaml`, `configs/node3.yaml` e il test di integrazione `tests/integration/cluster_convergence_test.go`. La sezione Compose riportava ancora una combinazione `sum/sum/average` non più coerente con il runtime reale; l'ho quindi aggiornata per documentare esplicitamente che tutti e tre i nodi usano `aggregation: average` con `initial_value` rispettivamente `10`, `30`, `50`, in allineamento con README e scenario M09.

## 2026-03-23 23:05:00 UTC
- **Descrizione task**: Chiarimento delle sezioni `Prerequisiti` e `Porte usate` in `docs/deployment.md` per distinguerle tra porte interne alla rete Compose e porte host non pubblicate nel Compose canonico.
- **File modificati**: `docs/deployment.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho confrontato il testo del documento con il `docker-compose.yml` di root, verificando che i servizi `node1`, `node2`, `node3` siano collegati solo alla rete `sdcc-net` e che il Compose canonico non definisca alcuna sezione `ports:`. Ho quindi rimosso il prerequisito fuorviante sulle porte UDP host libere e ho separato esplicitamente le porte runtime interne al cluster (`7001/7002/7003`) dagli eventuali scenari futuri o manuali in cui potrebbe esistere una pubblicazione host-side.
## 2026-03-23 23:20:00 UTC
- **Descrizione task**: Allineamento della sezione di testing sul bootstrap via join endpoint reale al comportamento effettivo della suite di integrazione.
- **File modificati**: `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho aggiornato la documentazione per descrivere il flusso reale del test `TestNodeBootstrapViaJoinEndpointPopulatesInitialMembership`, esplicitando build del binario temporaneo da `./cmd/node`, esecuzione via `exec.CommandContext(...)` con `--config` su file temporaneo, override `OBSERVABILITY_ADDR=127.0.0.1:0` e doppia evidenza osservabile richiesta: `JoinRequest` HTTP + almeno un payload gossip UDP verso il peer restituito nella `JoinResponse`.

## 2026-03-23 23:00:28 UTC
- **Descrizione task**: Normalizzazione della membership gossip per promuovere alias seed `host:port` alla forma canonica `node_id` e filtrare digest obsoleti.
- **File modificati**: `internal/membership/membership.go`, `internal/gossip/engine.go`, `tests/membership/bootstrap_test.go`, `tests/gossip/reexport.go`, `tests/gossip/membership_convergence_test.go`, `README.md`, `docs/architecture.md`, `docs/configuration.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho analizzato il flusso bootstrap/gossip esistente e ho introdotto una promozione esplicita dei placeholder seed tramite `TouchOrUpsertCanonical`, cosi' il primo heartbeat o digest che rivela il `node_id` reale unifica il record `host:port` con il peer canonico. Ho inoltre filtrato i digest membership per non ripropagare alias gia' superseduti e ho fissato test mirati per bootstrap seed-only, heartbeat canonico, sostituzione dell'alias e assenza di nuove transizioni `suspect/dead` dovute al placeholder dopo la normalizzazione.

## 2026-03-23 23:12:05 UTC
- **Descrizione task**: Correzione dell'algoritmo gossip `average` per preservare il contributo locale originario del nodo ed eliminare il drift verso la media corrente nei round successivi.
- **File modificati**: `internal/types/types.go`, `internal/gossip/engine.go`, `internal/gossip/state.go`, `cmd/node/main.go`, `tests/aggregation/average/average_convergence_test.go`, `tests/gossip/engine_test.go`, `tests/gossip/state_test.go`, `README.md`, `docs/architecture.md`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho introdotto nello stato gossip un campo runtime separato (`LocalValue`) per conservare il valore locale originario del nodo, inizializzato dal bootstrap con `cfg.InitialValue` e usato da `prepareLocalStateForRound(...)` per `average` al posto della stima aggregata corrente. Ho poi rimosso la reinferenza del contributo remoto da `remote.Value` quando il payload include già metadata average completi, lasciando al merge il solo aggiornamento dei contributi per nodo e il ricalcolo di `local.Value` come derivata dell'insieme dei contributi. Infine ho aggiunto regressioni mirate su convergenza a tre nodi (`10/30/50 -> 30`) e su assenza di drift del contributo locale dopo round multipli, riallineando anche la documentazione canonica.

## 2026-03-23 23:28:47 UTC
- **Descrizione task**: Rimozione dei path assoluti hardcoded dal test di integrazione del bootstrap via join endpoint usando la root repository calcolata dinamicamente.
- **File modificati**: `tests/integration/join_endpoint_bootstrap_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho verificato l'approccio già adottato in `tests/integration/compose_harness_test.go` e ho riallineato il test `join_endpoint_bootstrap` all'helper `integrationRepoRoot(t)` tramite una piccola helper locale riusata sia per la build di `./cmd/node` sia per l'esecuzione del binario con `--config <tempfile>`. In questo modo il test resta portabile e non assume più il path assoluto `/workspace/SDCC-project`.

## 2026-03-23 23:35:40 UTC
- **Descrizione task**: Rafforzamento della prerequisizione ambientale dei test Compose reali per verificare anche la disponibilità effettiva di `docker compose`.
- **File modificati**: `tests/integration/compose_harness_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Dopo aver riletto documentazione, architettura, configurazione e log operativi del repository, ho esteso `requireDocker()` mantenendo la semantica di skip ambientale: oltre a `exec.LookPath("docker")` e `docker info`, il test ora esegue anche `docker compose version` e, in caso di errore, skippa la suite reale con un messaggio diagnostico che espone anche l'output del comando per distinguere plugin mancante, permessi o problemi del client/daemon.

## 2026-03-23 23:50:00 UTC
- **Descrizione task**: Rafforzamento diagnostico di `scripts/cluster_up.sh` per rendere più leggibili i fallimenti di `docker compose up -d --build` e aggiornamento della guida testing sugli helper Compose.
- **File modificati**: `scripts/cluster_up.sh`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Dopo aver riletto documentazione canonica, architettura, configurazione, deployment, testing e log operativo, ho mantenuto invariato il cleanup preventivo di `cluster_up.sh` ma ho sostituito l'invocazione secca di `run_compose up -d --build` con un wrapper che cattura l'output, stampa il comando Compose effettivamente usato, raccoglie `docker compose ps` e un tail dei log di `node1`/`node2`/`node3`, quindi classifica esplicitamente i casi piu' comuni di triage (`plugin compose assente o non funzionante`, `build immagine fallita`, `container avviati ma unhealthy`). Ho infine aggiornato `docs/testing.md` per documentare il nuovo comportamento osservabile degli helper script Compose.

## 2026-03-24 00:01:27 UTC
- **Descrizione task**: Refactor di `scripts/cluster_common.sh` per eliminare uso di `mapfile` in `run_compose` e mantenere invocazione Compose diretta/portabile, con verifica compatibilità degli script cluster/fault injection.
- **File modificati**: `scripts/cluster_common.sh`, `docs/deployment.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho rimosso la funzione `compose_cmd` e la costruzione array con `mapfile`, sostituendo `run_compose` con invocazione diretta `docker compose -p "${PROJECT_NAME}" -f "${COMPOSE_FILE}" "$@"` mantenendo invariati `PROJECT_NAME` e `COMPOSE_FILE`. Ho verificato che gli script che riusano `run_compose` non richiedano ulteriori cambiamenti tramite ricerca riferimenti e validazione sintattica `bash -n`; ho inoltre aggiornato i prerequisiti di deployment indicando compatibilità Bash 3.2+ per gli script operativi.

## 2026-03-24 00:04:28 UTC
- **Descrizione task**: Revisione del cleanup preventivo in `scripts/cluster_up.sh` per mantenere il comportamento non bloccante senza sopprimere completamente stderr e con diagnostica esplicita in caso di errore.
- **File modificati**: `scripts/cluster_up.sh`, `docs/operational_log.md`.
- **Reasoning summary**: Ho sostituito la redirezione totale a `/dev/null` con una gestione esplicita dell'errore su `docker compose down --remove-orphans`: il comando resta non bloccante tramite branch con `true`, ma stderr resta visibile e viene aggiunto un messaggio `cleanup ignorato: ...` con exit code per migliorare il debug. In questo modo i fallimenti non vengono più silenziati e la causa resta osservabile nei test.

## 2026-03-24 00:15:02 UTC
- **Descrizione task**: Aggiornamento del file Compose canonico rimuovendo la direttiva `version: "3.9"` per allinearsi al formato Compose attuale e verifica del parsing con `docker compose config`.
- **File modificati**: `docker-compose.yml`, `docs/operational_log.md`.
- **Reasoning summary**: Ho rimosso la chiave `version` obsoleta mantenendo invariata la struttura `x-node-base`, `services` e `networks`; quindi ho verificato che il comando di validazione Compose continui a produrre configurazione valida senza warning legati alla versione.

## 2026-03-24 00:17:50 UTC
- **Descrizione task**: Aggiornamento di `scripts/cluster_up.sh` con fase di cleanup best-effort pre-avvio per rimuovere eventuali container legacy `sdcc-node1`, `sdcc-node2`, `sdcc-node3` non necessariamente appartenenti al project Compose corrente.
- **File modificati**: `scripts/cluster_up.sh`, `docs/operational_log.md`.
- **Reasoning summary**: Ho introdotto una routine esplicita che usa `docker ps -a --filter name=... -q` e `docker rm -f ...` per ogni nome legacy, mantenendo comportamento best-effort: in caso di errore di interrogazione/rimozione viene stampata diagnostica chiara su stderr e il flusso continua fino al successivo `run_compose up -d --build`.

## 2026-03-24 00:23:22 UTC
- **Descrizione task**: Rimozione dei campi `container_name` dai servizi `node1`, `node2`, `node3` nel Compose canonico per lasciare a Docker Compose la generazione automatica dei nomi container con project name `sdcc-bootstrap`; verifica del compose harness di integrazione rispetto alla risoluzione container via `docker compose ... ps -q <service>`.
- **File modificati**: `docker-compose.yml`, `docs/operational_log.md`.
- **Reasoning summary**: Ho eliminato i nomi container hardcoded per evitare dipendenze globali da identificativi fissi e ho verificato che `tests/integration/compose_harness_test.go` usi già la risoluzione dinamica per servizio (`docker compose -p sdcc-bootstrap -f docker-compose.yml ps -q <service>`), mantenendo il test indipendente dai nomi concreti dei container.

## 2026-03-24 10:45:31 UTC
- **Descrizione task**: Hardening heartbeat implicito gossip per evitare fallback a `node_id` come endpoint, con fallback sicuro su metadati transport e test regressivo su digest senza self-entry.
- **File modificati**: `internal/gossip/engine.go`, `internal/transport/transport.go`, `internal/transport/udp_transport.go`, `tests/gossip/engine_transport_contract_test.go`, `docs/architecture.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho reso `resolveOriginAddr` conservativo (solo endpoint `host:port` validi da digest o context transport), evitando promozioni alias non affidabili; in assenza di endpoint valido `markPeerAlive` ora tocca solo peer esistenti senza alterare `Addr`, prevenendo regressioni a `Addr=node_id`. Ho aggiunto un test che simula messaggio sano senza entry self nel digest e verifica sia la stabilità dell'endpoint canonico sia l'assenza di degrado `suspect/dead` dopo heartbeat implicito.

## 2026-03-24 10:57:15 UTC
- **Descrizione task**: Introduzione filtro esplicito del self node nel wiring gossip/membership: registrazione stabile di `selfNodeID`, esclusione del nodo locale da timeout transitions, digest membership e merge remoto, con test dedicati su membership/gossip e aggiornamento documentazione.
- **File modificati**: `internal/membership/membership.go`, `internal/gossip/engine.go`, `tests/membership/membership_test.go`, `tests/gossip/reexport.go`, `tests/gossip/membership_convergence_test.go`, `tests/gossip/engine_test.go`, `docs/architecture.md`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho reso esplicita la nozione di nodo locale nel set membership e nell'engine gossip per prevenire transizioni auto-indotte (`alive->suspect->dead`) e contaminazioni del digest/merge con entry self; ho aggiunto test regressivi che congelano sia il comportamento dati sia l'assenza di eventi/log timeout per il nodo locale, mantenendo coerenza tra implementazione e documentazione.

## 2026-03-24 18:56:18 UTC
- **Descrizione task**: Estensione del filtro self in `mergeMembership` per ignorare anche alias/endpoint locali e aggiunta di regressioni su digest remoto che include il nodo locale come `node3:7003`.
- **File modificati**: `internal/gossip/engine.go`, `tests/gossip/reexport.go`, `tests/gossip/membership_convergence_test.go`, `tests/gossip/engine_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho esteso il merge membership per scartare entry auto-riferite non solo tramite `node_id` canonico, ma anche tramite endpoint/alias locali noti (incluso `advertise_addr`), così da evitare la reintroduzione del nodo locale come peer remoto. Ho aggiunto test dedicati sia al merge (`TestMergeMembershipIgnoraEntryDelNodoLocale`) sia al ciclo round/logging (`TestRoundNonLoggaMembershipTransitionPerAliasDelNodoLocale`) per congelare il caso regressivo `peer_id=node3:7003` e verificare l'assenza di `membership_transition` auto-riferite.

## 2026-03-24 00:00:00 UTC
- **Descrizione task**: Hardening della risoluzione endpoint origine gossip per evitare fallback su `remoteAddr` non canonico, aggiornamento UDP transport con invio via socket locale persistente e introduzione regressioni su alias effimeri/membership_entries.
- **File modificati**: `internal/gossip/engine.go`, `internal/transport/udp_transport.go`, `tests/gossip/engine_transport_contract_test.go`, `tests/transport/udp_transport_test.go`, `tests/integration/cluster_convergence_test.go`, `docs/architecture.md`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho reso affidabile l'endpoint origine usando priorità metadata (`origin_addr`) e digest membership canonico, limitando l'uso di `remoteAddr` ai soli endpoint già dichiarati; in `markPeerAlive` ho bloccato upsert canonical con indirizzi non validati per evitare alias dead. Sul transport UDP ho privilegiato la socket persistente per stabilizzare la porta sorgente, mantenendo fallback compatibile. Ho aggiunto test mirati per regressione del fallback non canonico, metadata origine, stabilità membership su cluster 3 nodi e source port persistente.

## 2026-03-24 19:24:13 UTC
- **Descrizione task**: Aggiornamento canonicalizzazione heartbeat gossip per promuovere correttamente i placeholder bootstrap `node_id == addr` quando arriva `origin_id` canonico con `origin_addr` noto, con aggiunta di test negativo su endpoint non riconosciuto.
- **File modificati**: `internal/gossip/engine.go`, `tests/gossip/membership_convergence_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho esteso la validazione `isKnownCanonicalOrigin` per accettare anche il caso alias bootstrap basato su stesso `addr` (placeholder `host:port`) mantenendo invariato il ramo `origin_addr == ""` in `markPeerAlive`; ho aggiunto una regressione che verifica la mancata promozione quando `origin_addr` non combacia con alcun peer locale conosciuto.

## 2026-03-24 19:45:50 UTC
- **Descrizione task**: Hardening bootstrap/merge membership self-identity: seed del peer self canonico all’avvio, inclusione stabile di `advertise_addr` negli alias locali e regressione Compose su `membership_transition` riferite a self alias.
- **File modificati**: `cmd/node/main.go`, `internal/gossip/engine.go`, `tests/integration/cluster_convergence_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho anticipato la registrazione del peer locale canonico (`node_id` + `advertise_addr`) prima dell’engine gossip per evitare alias self effimeri; nel filtro merge ho mantenuto confronto normalizzato sia su `entry.NodeID` sia su `entry.Addr` e ho reso `collectSelfIdentityAliases` resiliente includendo sempre l’`advertise_addr` noto anche senza self nello snapshot. Ho aggiunto una regressione E2E Compose che fallisce se i log mostrano `membership_transition` dove `peer_id` coincide con l’alias locale del servizio.

## 2026-03-24 19:50:38 UTC
- **Descrizione task**: Hardening della canonicalizzazione membership in percorso heartbeat senza `origin_addr`, pulizia placeholder in `TouchOrUpsertCanonical`, filtro digest da snapshot sporchi e regressione di convergenza multi-round su assenza `node_id==addr` finale.
- **File modificati**: `internal/gossip/engine.go`, `internal/membership/membership.go`, `tests/membership/bootstrap_test.go`, `tests/gossip/membership_convergence_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho mantenuto il comportamento conservativo su endpoint non validati e, nel ramo `origin_addr==""` di `markPeerAlive`, ho aggiunto un riallineamento sicuro degli alias già presenti sullo stesso endpoint canonico locale (solo touch/promozione di entry esistenti, senza creare nuovi endpoint). In membership ho reso `TouchOrUpsertCanonical` idempotente rispetto a snapshot incoerenti eliminando sempre il placeholder `node_id==addr` quando si promuove al `node_id` canonico. Ho aggiunto test mirati per validare digest su snapshot con canonical+placeholder e una regressione integrata multi-round che verifica la convergenza senza placeholder finali per nodi già canonicalizzati.

## 2026-03-24 19:57:03 UTC
- **Descrizione task**: Hardening heartbeat implicito su merge gossip con debug logging branch-aware, consolidamento metadata `origin_addr` e regressioni su assenza intermittente metadata durante merge consecutivi.
- **File modificati**: `internal/gossip/engine.go`, `tests/gossip/reexport.go`, `tests/gossip/engine_transport_contract_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho esteso `markPeerAlive` con debug log gated da livello `debug` includendo `origin_id`, `origin_addr` risolto, peer toccato/promosso e motivo del ramo eseguito, mantenendo invariata la semantica conservativa di canonicalizzazione. Ho verificato la costruzione metadata esponendo `buildMessageMetadata` ai test e aggiunto regressioni per garantire `origin_addr` valorizzato quando il peer self canonico è noto nello snapshot; inoltre ho aggiunto uno scenario end-to-end nel contratto transport che simula merge ricevuti con metadata intermittente assente, verificando che il mapping canonico non venga perso e che il peer appena visto non transizioni a `suspect/dead`.

## 2026-03-24 20:24:12 UTC
- **Descrizione task**: Revisione del ramo `cmp == 0 && !samePayload` in `applyRemote(...)` per distinguere conflitti reali da divergenze solo metadata e aggiornamento test gossip correlati.
- **File modificati**: `internal/gossip/state.go`, `tests/gossip/state_test.go`, `docs/operational_log.md`.
- **Reasoning summary**: Ho introdotto un confronto semantico dedicato per `average/min/max` distinto dal confronto stretto già esistente, usando nel ramo `cmp==0` una classificazione `skipped` con reason `same_version_semantically_equivalent` quando il risultato aggregato è equivalente e i metadata sono compatibili. Ho mantenuto `same_version_different_payload` come `conflict` solo per divergenze che cambiano la semantica di convergenza (es. media incompatibile a parità versione), e ho esteso i test in `tests/gossip` con uno scenario concorrente `node-1/node-3` a stessa versione ma convergenza già raggiunta.

## 2026-03-24 20:27:49 UTC
- **Descrizione task**: Correzione semantica dei campi log `remote_merge` nell'handler di `Start` dell'engine gossip, con separazione esplicita tra peer locali e dimensione membership remota; aggiornamento documentazione observability e test di regressione dedicato.
- **File modificati**: `internal/gossip/engine.go`, `tests/gossip/engine_test.go`, `docs/observability.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho mantenuto `membership_entries` ancorato a `len(msg.Membership)` (payload remoto) e introdotto il conteggio locale post-merge (`localPeers := len(e.Membership.Snapshot())`) usato nel campo `peers`, così i log distinguono chiaramente vista locale runtime e cardinalità del digest ricevuto. Ho aggiunto documentazione esplicita della semantica e un test che verifica la divergenza intenzionale tra i due campi.

## 2026-03-24 20:39:40 UTC
- **Descrizione task**: Aggiornamento estrazione shutdown in `cluster_collect_results.sh` con deduplica deterministica per `node_id` e allineamento documentazione testing su contenuto univoco di `latest-final-values.txt`.
- **File modificati**: `scripts/cluster_collect_results.sh`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho sostituito la scansione lineare dei log con una pipeline che filtra `shutdown nodo completato`, estrae `node_id`/`time` e mantiene per ogni nodo solo la riga temporalmente più recente (tie-break sull'ultima occorrenza nel file), poi ordina deterministicamente per `node_id` prima di scrivere `final-values-*.txt`. Ho aggiornato la sezione testing per esplicitare che `latest-final-values.txt` contiene un record finale univoco per nodo coerente con la nuova logica di deduplica.

## 2026-03-24 20:42:54 UTC
- **Descrizione task**: Arricchimento del wiring logger runtime con identificativo stabile di istanza (`runtime_instance`) da env `HOSTNAME`, propagazione del logger arricchito in bootstrap/transport/engine/shutdown e aggiornamento documentazione observability.
- **File modificati**: `cmd/node/main.go`, `docs/observability.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho introdotto in `main.go` una risoluzione esplicita dell'identità istanza (`HOSTNAME` con fallback `node_id`/`unknown`) e creato subito dopo `NewLogger(...)` un logger arricchito via `With("runtime_instance", ...)`, riusato in tutto il wiring runtime incluso il logger passato a `gossip.NewEngine`, così i log di bootstrap, transport, merge remoto, round e shutdown condividono automaticamente lo stesso campo stabile. Ho poi aggiornato `docs/observability.md` per documentare la semantica operativa del nuovo campo e il suo fallback.

## 2026-04-03 10:17:30 UTC
- **Descrizione task**: Creazione della guida demo operativa `docs/demo.md` con sezioni richieste (scopo, prerequisiti reali, setup/avvio, osservazioni, criteri misurabili, supporto crash/restart, troubleshooting) e allineamento dei riferimenti incrociati con README e testing canonico.
- **File modificati**: `docs/demo.md`, `README.md`, `docs/testing.md`, `docs/operational_log.md`.
- **Reasoning summary**: Ho introdotto un documento demo dedicato basato esclusivamente su comandi, test e script realmente presenti nel repository (Compose canonico root, `TestClusterConvergence`, `TestNodeCrashAndRestart`, `scripts/fault_injection/*`), evitando workflow inventati e aggiungendo collegamenti bidirezionali per mantenere coerenza tra documentazione operativa e strategia di test.
