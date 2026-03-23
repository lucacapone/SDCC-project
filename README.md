# SDCC-project

Progetto SDCC per aggregazione dati distribuita con approccio **gossip decentralizzato**.

## Indice
- [Panoramica sistema gossip decentralizzato](#panoramica-sistema-gossip-decentralizzato)
- [Architettura ad alto livello](#architettura-ad-alto-livello)
- [Scelte architetturali confermate](#scelte-architetturali-confermate)
- [Protocollo gossip (M01)](#protocollo-gossip-m01)
- [Stato avanzamento milestone](#stato-avanzamento-milestone)
- [Sezione aggregazioni](#sezione-aggregazioni)
- [Configurazione esterna](#configurazione-esterna)
- [Avvio locale con Docker Compose](#avvio-locale-con-docker-compose)
- [Esecuzione test](#esecuzione-test)
- [Test interni di convergenza in-memory](#test-interni-di-convergenza-in-memory)
- [Test di integrazione end-to-end M09](#test-di-integrazione-end-to-end-m09)
- [Script/comandi standard](#scriptcomandi-standard)
- [Supporti operativi fault injection](#supporti-operativi-fault-injection)
- [Criteri di successo misurabili](#criteri-di-successo-misurabili)
- [Demo rapida](#demo-rapida)
- [Nota deploy EC2 essenziale](#nota-deploy-ec2-essenziale)

## Panoramica sistema gossip decentralizzato
Il sistema è pensato per nodi indipendenti che scambiano periodicamente informazioni in modalità peer-to-peer.

## Architettura ad alto livello
Ogni nodo usa configurazione esterna (YAML/JSON + variabili ambiente), costruisce una membership locale dai seed peer e avvia round gossip periodici con intervallo configurabile. Il parametro `fanout` è già in configurazione ma nel runtime corrente non è ancora applicato alla selezione peer (invio verso tutti i peer non `dead/leave`).

## Scelte architetturali confermate
- **Transport tra nodi**: UDP + payload JSON (`[]byte`) su adapter `Transport`.
- **Strategia gossip (implementata oggi)**: push verso peer attivi (`alive`/`suspect`) con payload completo stato+membership; fanout variabile pianificato ma non ancora attivo nel loop runtime.
- **Aggregazioni richieste**: `sum`, `average`, `min`, `max`.
- **Membership/discovery**: join endpoint con fallback su seed statici da configurazione.

Queste scelte sono definitive per il progetto corrente e sostituiscono la precedente matrice comparativa.

## Decisioni confermate (2026-03-05)
- **Transport**: UDP adapter concreto (`internal/transport/udp_transport.go`) con fallback esplicito a `NoopTransport` solo in caso di errore di init.
- **Strategia gossip**: round periodici push su `Transport` astratto; selezione fanout/retry non ancora implementata (presente TODO tecnico nel codice engine).
- **Aggregazioni richieste**: **sum + average + min/max**.

## Protocollo gossip (M01)
Sintesi operativa del protocollo M01:
- `GossipMessage` include i campi principali `message_id`, `origin_node`, `state_version` (con `version_epoch` + `version_counter`), `payload`, `sent_at` e `membership` (digest serializzato con `status` + `incarnation` per peer).
- Il versioning è composto da `version_epoch + version_counter`: l'epoch separa i cicli/logical reset, il counter ordina gli aggiornamenti nello stesso epoch.
- Regole principali di merge: `duplicate_message_id` (idempotenza), `out_of_order_stale` (scarto update vecchi), `same_version_different_payload` (conflitto a parità versione) e `remote_newer_version` (applicazione update più recente).
- Comando mirato di verifica: `go test ./tests/gossip -run TestMergeRules -count=1`.

Per i dettagli completi consultare l'architettura: [docs/architecture.md](docs/architecture.md).
- Test membership dedicati: `go test ./tests/gossip -run TestMergeMembershipConvergeConDuplicatiOutOfOrder`.

## Stato avanzamento milestone
- **M01**: completata (contratto messaggio gossip, versioning `epoch+counter`, merge deterministico e test di convergenza base).
- **M02**: completata a livello repository su modello membership locale + propagazione digest gossip + merge `incarnation/status` + test dedicati.
- **M03**: completata lato documentazione del transport astratto/concreto, confini gossip↔adapter e contratto verificato da suite dedicata.
- **M04**: completata lato repository per `sum` (algoritmo base in `internal/aggregation/sum/`, merge gossip idempotente con contributi/versioni per nodo in `internal/gossip/state.go`, gestione duplicati/out-of-order, saturazione overflow e suite canonica di convergenza in `tests/aggregation/sum/sum_convergence_test.go`).
- **M05**: completata lato repository/documentazione per estensione e consolidamento `average`/`min`/`max`, regressione multi-aggregazione e verifica coerenza architetturale.
- **M08**: completata come milestone di consolidamento test/documentazione; copertura iniziale esplicitata per `merge`, `membership`, `config`, `aggregation` e comando unico di verifica post-milestone introdotto nel README.
- **M09**: completata lato test/documentazione con suite canonica `tests/integration/TestClusterConvergence`, documento `docs/testing.md` e comando operativo ufficiale dedicato alla convergenza cluster.
- **M10**: completata lato repository/documentazione con test canonico `tests/integration/TestNodeCrashAndRestart`, criteri osservabili di crash/restart in `docs/testing.md` e task report dedicato `docs/task/M10.md`.
- **M11**: completata lato documentazione operativa dell'observability con guida dedicata `docs/observability.md`, task report `docs/task/M11.md` e comando canonico di verifica `go test ./tests/observability -run TestMetricsExposure`.

Comandi di verifica milestone:
- M03 → `go test ./tests/transport -run TestTransportContract`
- M04 → `go test ./tests/aggregation/sum -run TestSumConvergence`
- M05 → test merge `average`/`min`/`max` + regressione multi-aggregazione (vedi sezione test M05).
- M08 → `go test ./... -run Test -count=1`
- M11 → `go test ./tests/observability -run TestMetricsExposure`

Documento task:
- `docs/task/M01.md`
- `docs/task/M02.md`
- `docs/task/M03.md`
- `docs/task/M04.md`
- `docs/task/M05.md`
- `docs/task/M06.md`
- `docs/task/M07.md`
- `docs/task/M08.md`
- `docs/task/M09.md`
- `docs/task/M10.md`
- `docs/task/M11.md`

## Raccomandazione membership / discovery
Consiglio **Opzione B (join endpoint) con fallback seed statici da configurazione**.

Perché questa scelta è la più equilibrata per il progetto:
- mantiene il sistema decentralizzato per il calcolo degli aggregati;
- consente join dinamici (elasticità) senza aggiornare manualmente tutti i file di configurazione;
- resta semplice da testare in locale e su EC2 perché i seed rimangono piano B operativo.

Impatto pratico previsto:
- `join_endpoint` è già presente in configurazione come meccanismo di bootstrap opzionale;
- nel runtime reale (`cmd/node`) il nodo usa un client HTTP concreto verso `http://<join_endpoint>/join`; se il join fallisce o non è configurato, resta attivo il fallback su `bootstrap_peers`/`seed_peers`;
- la membership operativa resta decentralizzata e evolve via gossip peer-to-peer.

## Sezione aggregazioni
Aggregazioni abilitate via configurazione:
- `sum`
- `average`
- `min`
- `max`

La chiave `aggregation` seleziona l'aggregazione attiva nel nodo, validata contro `enabled_aggregations`.
La configurazione segue due livelli:
- `enabled_aggregations`: insieme delle aggregazioni consentite per il nodo (whitelist runtime);
- `aggregation`: aggregazione effettivamente attiva nel nodo e usata dal gossip locale.
La validazione fallisce se `aggregation` non appartiene a `enabled_aggregations`.
Il layer comune risiede in `internal/aggregation`, con implementazioni dedicate in `sum`, `average`, `min` e `max`.
- **Stato reale `sum`**: algoritmo base in `internal/aggregation/sum/`; il merge gossip usa `state.aggregation_data.sum` con contributi/versioni per nodo ed è implementato in `internal/gossip/state.go`, dove mantiene semantica idempotente su duplicati/out-of-order; la suite canonica di convergenza è `tests/aggregation/sum/sum_convergence_test.go` con `TestSumConvergence`.
- **Stato reale `average`**: merge gossip convergente con metadati `state.aggregation_data.average` (`contributions.sum/count` + `versions` per nodo), evitando la deriva della media pairwise.
- **Stato reale `min`/`max`**: merge gossip monotono robusto con metadati opzionali `state.aggregation_data.min/max.versions` per nodo e fallback retrocompatibile su payload legacy senza metadati.
- Overflow numerico in `sum`: saturazione esplicita a `±math.MaxFloat64` con flag `overflowed` propagato nello stato gossip.

## Observability minima
Lo stato post-M11 dell'observability è documentato in modo canonico in `docs/observability.md`.

**Decisione architetturale vincolante**: per il repository la scelta univoca è la **soluzione ibrida** — **stdout strutturato** per gli eventi applicativi e **HTTP** per metriche/probe. I task successivi non devono introdurre alternative concorrenti o duplicare la stessa informazione su superfici osservabili diverse senza aggiornamento esplicito della documentazione canonica.

Sintesi operativa M11:
- architettura minima composta da logger strutturato, collector metriche, stato lifecycle del nodo e server HTTP minimo integrato in `cmd/node/main.go`;
- campi log stabili per gli eventi gossip principali (`event`, `node_id`, `round`, `peers`, `estimate`, `result`, `node_state`), emessi su stdout/stderr strutturato;
- metriche e probe esposte via endpoint HTTP `/health`, `/ready` e `/metrics`;
- binding HTTP configurabile via `OBSERVABILITY_ADDR` (default `:8080`);
- criterio canonico di readiness: `/ready` resta `503` fino a bootstrap completato + engine gossip avviato, poi passa a `200`;
- lifecycle del server HTTP: avvio insieme al runtime del nodo, disponibilità per tutta la vita del processo e terminazione contestuale allo shutdown;
- comando canonico di verifica post-M11:
  - `go test ./tests/observability -run TestMetricsExposure`

Per istruzioni d'uso, verifica manuale, limiti noti e scelte progettuali consultare direttamente `docs/observability.md`.

## Configurazione esterna
Documento canonico della configurazione runtime:
- `docs/configuration.md`

File di esempio:
- `configs/example.yaml`

Parametri esterni principali:
- `join_endpoint`
- `bootstrap_peers`
- `gossip_interval_ms`
- `fanout`
- `node_port`
- `advertise_addr`
- `seed_peers`
- `membership_timeout_ms`
- `enabled_aggregations`

Esecuzione locale con file config:
```bash
go run ./cmd/node --config configs/example.yaml
```

Override via variabili ambiente (precedenza sull'YAML):

> Nota runtime reale: gli override env numerici o CSV malformati non fanno fallire `Load`; vengono ignorati esplicitamente e il nodo mantiene il valore già ottenuto da default/file. Esempi congelati dalla suite: `NODE_PORT=abc`, `FANOUT=abc`, `ENABLED_AGGREGATIONS=sum,,max`, `BOOTSTRAP_PEERS=node-1:7001,`.

```bash
NODE_ID=node-custom \
NODE_PORT=7100 \
ADVERTISE_ADDR=node-local-1:7100 \
JOIN_ENDPOINT=bootstrap:9000 \
BOOTSTRAP_PEERS=node-1:7001,node-2:7002 \
SEED_PEERS=node-1:7001,node-2:7002 \
GOSSIP_INTERVAL_MS=500 \
FANOUT=1 \
MEMBERSHIP_TIMEOUT_MS=3000 \
ENABLED_AGGREGATIONS=sum,average,min,max \
AGGREGATION=min \
go run ./cmd/node --config configs/example.yaml
```

Flusso bootstrap all'avvio:
- il nodo invia al bootstrap una `JoinRequest` con `node_id` logico e `addr` reale (`host:port`) ricavato da `advertise_addr` oppure, in fallback locale, da `bind_address:node_port`;
- il nodo prova `join_endpoint` per ottenere snapshot/delta membership iniziale;
- se il join non è disponibile, usa `bootstrap_peers` (o `seed_peers` come fallback compatibile) come elenco di endpoint reali `host:port`;
- eventuali seed placeholder creati dal fallback vengono riallineati al vero `node_id` non appena il peer remoto propaga la membership completa;
- il bootstrap non è autoritativo: dopo discovery iniziale la membership evolve solo via gossip peer-to-peer.

Convenzione unica adottata:
- `node_id` = identificatore logico stabile del nodo (`node-1`, `node-2`, ...);
- `addr` = endpoint di rete realmente raggiungibile nel formato `host:port`;
- nei deployment Docker Compose il `host` dell'endpoint coincide con il **service name** (`node1`, `node2`, `node3`), che Docker risolve via DNS interno.

## Avvio locale con Docker Compose
Per M07 il file Compose canonico del cluster locale è:
- `docker-compose.yml` alla root della repository.

Il file `deploy/docker-compose.yml` resta solo come **variante secondaria/storica di promemoria** e non va usato come sorgente operativa principale, così da evitare ambiguità sul file Compose da eseguire.

Comandi reali del flusso standard:
```bash
docker compose up -d --build
docker compose ps
docker compose logs -f node1
docker compose down
```

Ogni servizio usa la stessa immagine applicativa locale costruita dal `Dockerfile` multi-stage e monta una config esterna dedicata:
- `configs/node1.yaml`
- `configs/node2.yaml`
- `configs/node3.yaml`

I nodi si scoprono tramite la rete Compose `sdcc-net` e i nomi servizio `node1`, `node2`, `node3`: questi hostname vengono risolti via DNS interno di Compose e sono gli stessi usati negli `advertise_addr` e nei `seed_peers` del runtime.

I file `configs/node1.yaml`, `configs/node2.yaml`, `configs/node3.yaml` sono coerenti con il runtime effettivo perché dichiarano:
- `node_id` logici distinti (`node-1`, `node-2`, `node-3`);
- `advertise_addr` raggiungibili sulla rete Compose (`node1:7001`, `node2:7002`, `node3:7003`);
- peer seed espressi come endpoint reali `host:port` e non come identificativi logici.

Per passare configurazioni personalizzate basta cambiare i file montati. Il Compose canonico non duplica più `NODE_ID`, `NODE_PORT` o `SEED_PEERS` via environment, così da mantenere nei file YAML la sorgente di verità per identità logica ed endpoint pubblicizzati. La build dell'immagine avviene localmente tramite `docker compose up -d --build`, senza più usare `golang:1.22` con `go run` dentro i container.

Dettagli operativi canonici di build/deploy locale multi-nodo:
- `docs/deployment.md`

## Test interni di convergenza in-memory
Le suite storiche nel package `tests/gossip` restano utili come test interni di convergenza e resilienza in-memory, ma **non** rappresentano la suite canonica M09. Sono verifiche interne al repository rivolte alla logica gossip, senza Docker Compose e senza un cluster locale multi-nodo eseguito come scenario end-to-end.

Comandi interni disponibili:
```bash
go test ./tests/gossip -run TestIntegrationGossipConvergence -count=1
go test ./tests/gossip -run TestCrash -count=1
```

Target `Makefile` dedicati:
```bash
make test-integration-internal
make test-crash
make test-crash-restart
# alias equivalente: make test-m10
```

Differenza operativa tra i target crash:
- `make test-crash`: **target interno/debug** che resta puntato ai test in-memory del package `tests/gossip`.
- `make test-crash-restart` / `make test-m10`: **target canonico milestone M10** che esegue `tests/integration/TestNodeCrashAndRestart`.

## Test di integrazione end-to-end M09
Documento canonico dei test di integrazione e dei comandi operativi:
- `docs/testing.md`

Test canonico M09 disponibile:
- `tests/integration/cluster_convergence_test.go` (`TestClusterConvergence`)

Test canonico M10 disponibile:
- `tests/integration/node_crash_restart_test.go` (`TestNodeCrashAndRestart`)

Il target `make test-integration` punta ufficialmente alla suite di integrazione in `tests/integration`; nel repository la chiamiamo **suite di integrazione end-to-end M09** perché valida il comportamento osservabile del cluster a tre nodi come scenario black-box di milestone; allo stesso tempo l'harness usato dal test resta in-memory, quindi non sostituisce i controlli manuali su **cluster locale multi-nodo con Docker Compose**.

Sintesi criteri M09:
- scenario congelato a **3 nodi** (`node-1`, `node-2`, `node-3`);
- aggregazione attiva: `average`;
- valori iniziali: `10`, `30`, `50`;
- criterio di successo: banda cluster `max(values) - min(values) <= 0.05`;
- timeout esplicito: `350ms`, derivato da `gossip_interval = 10ms`, allowance di bootstrap `50ms` e buffer locale/CI `300ms`;
- report finale per nodo: `node_id`, `observed_value`, `expected_delta`, `common_band`.

Comando ufficiale M09:
```bash
go test ./tests/integration -run TestClusterConvergence -count=1
make test-integration
```

Comando ufficiale M10:
```bash
go test ./tests/integration -run TestNodeCrashAndRestart -count=1
make test-crash-restart
# alias equivalente: make test-m10
```

Distinzione esplicita dei target crash/restart:
- `make test-crash` continua a rappresentare il livello **interno/debug** del package `tests/gossip`.
- `make test-crash-restart` e `make test-m10` rappresentano il livello **canonico milestone M10** nella suite `tests/integration`.

Sintesi criteri M10:
- crash osservabile di **1 nodo su 3** durante round gossip già attivi;
- convergenza del **cluster residuo** (`node-2`, `node-3`) entro banda `<= 0.05` e stabilizzazione su più snapshot consecutivi;
- restart del nodo crashato con nuova registrazione sulla rete di test;
- **rejoin reale** verificato osservando che il nodo riavviato si allontana dal valore di restart artificiale;
- convergenza finale del nodo rientrato entro banda cluster `<= 0.08` dopo il rejoin.

Comando ufficiale M10:
```bash
go test ./tests/integration -run TestNodeCrashAndRestart -count=1
make test-crash-restart
# alias equivalente: make test-m10
```

## Esecuzione test
- Test transport (UDP + contratto): `go test ./tests/transport -count=1`
```bash
go test ./...
```

Comandi mirati membership (M02), separati per livello di verifica:
```bash
# verifica unitaria membership
go test ./tests/membership -run 'TestJoinLeave|TestTimeoutTransitions|TestPruneRemovesExpiredDeadPeerAndBlocksObsoleteReintroduction' -count=1

# verifica gossip membership
go test ./tests/gossip -run 'TestMergeMembership|TestRoundSerializzaMembershipConIncarnation' -count=1

# eventuale verifica integrazione runtime
go test ./tests/integration -run TestRuntimeMembershipFailureDetection -count=1
```

Comando operativo M04 (verifica convergenza `sum`):
```bash
go test ./tests/aggregation/sum -run TestSumConvergence
```

Comandi operativi M05:
```bash
# average: merge convergente per contributi/versioni per nodo
go test ./tests/gossip -run TestMergeAverageContributiConvergentiPerNodo

# min/max: merge monotono robusto e compatibilità payload legacy
go test ./tests/gossip -run 'TestMergeMinMonotonoGestisceStatoVuotoELegacy|TestMergeMaxMonotonoGestisceStatoVuotoELegacy'

# regressione multi-aggregazione: sum invariata con nuove aggregazioni abilitate
go test ./tests/gossip -run TestSumRegressionConNuoveAggregazioni
```

Comando di verifica post-M08:
```bash
go test ./... -run Test -count=1
```

Stato post-M08 dichiarato esplicitamente:
- area `merge`: suite dedicata nel package `tests/gossip` con verifica delle regole di merge, casi out-of-order, legacy, overflow e regressioni multi-aggregazione;
- area `membership`: suite dedicata nei package `tests/membership` e `tests/gossip` per join/leave, timeout, incarnation, bootstrap e convergenza digest membership;
- area `config`: suite dedicata in `tests/config` per load/validate, precedence env/file e validazioni bloccanti;
- area `aggregation`: suite root + test di convergenza per `sum`, `average`, `min`, `max`, con verifica finale repository-wide demandata al comando sopra riportato.

## Supporti operativi fault injection
Per validazione manuale e debug del cluster Docker Compose canonico è disponibile la directory `scripts/fault_injection/`, costruita come estensione leggera degli helper già presenti in `scripts/` e senza introdurre orchestrazione centralizzata o dipendenze fragili.

Script disponibili:
- `scripts/fault_injection/node_stop_start.sh`: simula `stop`, `start` o `bounce` di un singolo servizio Compose (`node1`, `node2`, `node3`) con parametri configurabili via argomenti o variabili ambiente.
- `scripts/fault_injection/collect_debug_snapshot.sh`: raccoglie snapshot diagnostici minimi (`docker compose ps`, log cluster, log del servizio target, `docker inspect`, metadata) in `artifacts/fault_injection/`.
- `scripts/fault_injection/common.sh`: helper condivisi che riusano `scripts/cluster_common.sh` per rimanere allineati al `docker-compose.yml` canonico di root.

Esempi rapidi:
```bash
# arresta e riavvia node2 con una breve pausa post-stop
AFTER_STOP_SLEEP_SECONDS=5 scripts/fault_injection/node_stop_start.sh bounce node2

# raccoglie uno snapshot diagnostico del nodo riavviato
SNAPSHOT_LABEL=post-restart scripts/fault_injection/collect_debug_snapshot.sh node2
```

Nota importante: il test automatico canonico di crash/restart continua a vivere in `tests/integration` come suite **in-memory**; gli script in `scripts/fault_injection/` sono supporti operativi/manuali per debug e validazione locale del cluster Compose, non dipendenze hard della suite Go.

## Script/comandi standard
È disponibile `Makefile` con target per esecuzioni riproducibili locali e Docker:

```bash
# Suite completa
make test

# Solo unit test (config + aggregation + membership)
make test-unit

# Test di integrazione/end-to-end M09
make test-integration

# Test interni di convergenza gossip in-memory
make test-integration-internal

# Robustezza crash/rejoin in-memory
make test-crash

# Esecuzione test completa dentro container Go
make docker-test
```

## Criteri di successo misurabili
I test introdotti in repository usano i seguenti criteri quantitativi:

1. **M09 — Convergenza gossip (3 nodi, harness in-memory della suite di integrazione end-to-end M09)**:
   - criterio esplicito di pass/fail: differenza massima tra stati `<= 0.05`
   - riferimento informativo nel report: media iniziale `30.0` per input `10`, `30`, `50`
   - timeout massimo `350ms`, coerente con la documentazione canonica M09 (`50ms` bootstrap + `300ms` buffer locale/CI).
2. **M10 — Crash di un nodo e convergenza del cluster residuo**:
   - crash di `1` nodo su `3` solo dopo avere osservato attività gossip reale pre-crash;
   - con `1` nodo down su `3`, il cluster residuo (`2/3`) converge con banda `<= 0.05`;
   - il cluster residuo deve mostrare progresso o stabilizzazione coerente su più snapshot consecutivi, non su un singolo campione;
   - timeout operativo della fase residua: `220ms`.
3. **M10 — Restart, rejoin e convergenza finale del nodo rientrato**:
   - il nodo crashato viene effettivamente deregistrato dal transport di test e poi nuovamente registrato al restart;
   - il nodo riavviato non resta bloccato sul valore di restart e deve quindi mostrare un **rejoin reale**;
   - il nodo rientrato converge poi nella banda finale del cluster con soglia `<= 0.08`;
   - il valore finale del nodo rientrato viene confrontato sia con la banda del cluster sia con un riferimento informativo derivato dal cluster residuo stabile;
   - timeout operativo della fase di rejoin/stabilizzazione finale: `320ms`.
4. **Validazione configurazione**:
   - parsing YAML/JSON corretto
   - errore obbligatorio su parametri non validi (`fanout <= 0`, `aggregation` non abilitata, peer `host:porta` malformati, `node_port` fuori range, duplicati o valori vuoti nelle liste).

## Demo rapida
```bash
# 1) Build immagine applicativa e avvio cluster dal file canonico `docker-compose.yml`
docker compose up -d --build

# 2) Verifica servizi e stato dei container
docker compose ps

# 3) Segui i log del nodo 1 per osservare bootstrap, gossip e discovery via service name Compose
docker compose logs -f node1

# 4) Arresto e rimozione del cluster locale
docker compose down
```

Durante la demo rapida i nodi si scoprono usando direttamente la rete Compose e i nomi servizio `node1`, `node2`, `node3`; questi service name compaiono negli `advertise_addr` e nei peer seed come endpoint reali `host:port`, mentre i `node_id` restano identificativi logici separati.

## Nota deploy EC2 essenziale
Checklist minima:
1. aprire security group solo sulle porte necessarie tra nodi;
2. usare Docker + Compose anche su EC2 per mantenere parità con locale;
3. configurare indirizzi peer con DNS privato/VPC;
4. abilitare log centralizzati (CloudWatch o equivalente) per osservare convergenza gossip.
