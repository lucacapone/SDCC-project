# Testing ed evaluation

## Strategia

La repository combina quattro livelli:

1. test unitari e contrattuali sui package pubblici attraverso wrapper sotto `tests/`;
2. test concorrenti e di integrazione in-memory, veloci e deterministici;
3. test black-box su processi e socket reali;
4. suite Docker Compose per il deployment effettivo a 3 e 6 nodi.

I test Compose richiedono Docker Engine e il plugin Compose. Non sono simulazioni: invocano gli script in `scripts/`, leggono metriche/log e raccolgono artefatti sotto `artifacts/cluster`.

## Comandi principali

```bash
# intera suite
go test ./... -count=1

# controllo delle race (più lento)
go test -race ./... -count=1

# analisi statica Go
go vet ./...

# nucleo unitario
make test-unit

# test d'integrazione selezionati dal prefisso TestClusterConvergence
make test-integration

# crash, continuità residua e restart/rejoin Compose
make test-crash-restart
```

`go test ./...` include anche i test Compose: senza Docker tali test falliscono o vengono saltati solo dove la singola suite prevede esplicitamente quella condizione. Per feedback senza Docker usare i comandi mirati in-memory indicati sotto.

## Copertura per area

### Configurazione

`tests/config` verifica default, YAML/JSON, precedence environment, errori di tipo, peer list, range porte, aggregazioni, advertise address, discovery e compatibilità tra timeout e copertura gossip.

```bash
go test ./tests/config -count=1
```

### Aggregazioni

`tests/aggregation` verifica factory e contratto numerico; le sottocartelle `sum`, `average`, `min`, `max` verificano convergenza, duplicati, fuori ordine, nodi lenti, filtro membership, leave/dead e rejoin. La suite average legge anche le sei configurazioni canoniche.

```bash
go test ./tests/aggregation/... -count=1
```

### Gossip e convergenza in-memory

`tests/gossip` copre envelope, versioning epoch/counter, deduplica, conflitti, merge CRDT-like, fanout rotante, heartbeat, canonicalizzazione dei peer, collector e concorrenza tra round/ricezione.

```bash
go test ./tests/gossip -count=1
go test ./tests/gossip -run TestIntegrationGossipConvergence -count=1
go test ./tests/gossip -run 'TestCrashNodeDownClusterResidualConverges|TestCrashRestartRejoinOptional' -count=1
```

### Membership e identity

`tests/membership` verifica join/leave, precedence degli stati, timeout, self filtering, tombstone, incarnation, rejoin, placeholder seed e accesso concorrente. `tests/identity` verifica allocazione e persistenza della generation.

```bash
go test ./tests/membership ./tests/identity -count=1
```

### Transport

`tests/transport` congela il contratto astratto e prova l'adapter UDP reale: start/send/close, context cancellato, doppio start e socket persistente. `tests/gossip/TestEngineUsaSoloInterfacciaTransportStartStop` verifica il confine engine/adapter.

```bash
go test ./tests/transport -count=1
go test ./tests/gossip -run 'TestEngineUsaSoloInterfacciaTransportStartStop|TestEngineGestisceMessaggiInIngressoViaHandlerTransport' -count=1
```

### Observability

`tests/observability` verifica logger, endpoint, formato metriche e transizioni lifecycle. I test gossip verificano anche aggiornamento runtime del collector e schema `convergence_sample`.

```bash
go test ./tests/observability -count=1
go test ./tests/gossip -run 'TestRoundAggiornaCollector|TestConvergenceSampleEventSchema' -count=1
```

### Pipeline di convergenza

`tests/convergence` verifica parsing dei log, normalizzazione temporale, oracle per le quattro aggregazioni, criterio di convergenza persistente, completezza del set nodi e SVG. `tests/convergence-chart` verifica l'orchestrazione CLI.

```bash
go test ./tests/convergence ./tests/convergence-chart -count=1
```

## Test d'integrazione

### Cluster a 3 nodi

`TestClusterConvergence` avvia il Compose root, attende readiness e verifica che le stime del cluster `average` convergano con banda `max-min <= 0.05`. `TestMembershipEntriesRestanoStabiliNelCluster3Nodi` controlla che non restino alias seed duplicati.

```bash
go test ./tests/integration -run 'TestClusterConvergence$|TestMembershipEntriesRestanoStabiliNelCluster3Nodi' -count=1
```

La variante deterministica senza Docker è:

```bash
go test ./tests/integration -run TestClusterConvergenceInMemory -count=1
```

### Cluster a 6 nodi

`TestClusterConvergenceScaleCompose` usa `deploy/docker-compose.scale.yml`; la variante in-memory usa otto nodi e misura convergenza con fanout limitato.

```bash
go test ./tests/integration -run TestClusterConvergenceScaleCompose -count=1
go test ./tests/integration -run TestClusterConvergenceScaleInMemory -count=1
```

### Crash, restart e rejoin

`TestNodeCrashAndRestart` arresta realmente `node1`, verifica che i nodi rimanenti continuino ad avanzare, attende la failure detection, riavvia il servizio e richiede membership/stima riconvergenti. Le generation persistenti consentono alla stessa `node_id` di superare lo stato precedente.

```bash
go test ./tests/integration -run TestNodeCrashAndRestart -count=1
```

Varianti veloci:

```bash
go test ./tests/integration -run TestNodeCrashAndRestartInMemory -count=1
go test ./tests/integration -run TestNodeCrashRestartSixNodesMembershipAwareAverage -count=1
```

### Failure detection, leave e bootstrap join

Sono presenti test runtime per `alive → suspect → dead`, una prova lunga senza falsi suspect, leave volontario con convergenza residua e bootstrap contro un join endpoint HTTP reale simulato dal test.

```bash
go test ./tests/integration -run 'TestRuntimeMembership|TestVoluntaryLeave|TestNodeBootstrapViaJoinEndpoint' -count=1
```

### Crash multipli e partizione

`TestSequentialCrashPartitionAndRejoin` orchestra due crash sequenziali, una disconnessione temporanea dalla rete Docker, recovery e rejoin, riusando gli script di fault injection.

```bash
go test ./tests/integration -run TestSequentialCrashPartitionAndRejoin -count=1
```

Lo stesso scenario può essere eseguito manualmente dopo lo startup:

```bash
scripts/fault_injection/scenario_sequential_crash_partition_rejoin.sh
```

## Osservazione e misura della convergenza

L'engine registra `event=convergence_sample` con timestamp, nodo, round, aggregazione e stima. Per una run a sei nodi:

```bash
SDCC_COMPOSE_FILE=deploy/docker-compose.scale.yml \
SDCC_PROJECT_NAME=sdcc-scale \
SDCC_SERVICES='node1 node2 node3 node4 node5 node6' \
scripts/cluster_convergence_report.sh
```

Lo script forza una run delimitata, raccoglie i log e genera CSV/SVG. Il tool calcola l'oracle dagli `initial_value`, normalizza l'asse temporale al primo campione globale e considera convergente il primo istante dal quale tutte le serie restano nella tolleranza. Rifiuta set di nodi incompleti o estranei.

Per uno snapshot senza polling del cluster scale già avviato:

```bash
SDCC_COMPOSE_FILE=deploy/docker-compose.scale.yml \
SDCC_PROJECT_NAME=sdcc-scale \
scripts/show_aggregation_status.sh
```

## Traffic Control

La modalità `deploy/docker-compose.tc.yml` applica profili Linux NetEm isolati e osserva la convergenza dei sei nodi:

```bash
bash -n deploy/traffic-control/entrypoint.sh scripts/demo_tc_latency.sh
scripts/demo_tc_latency.sh --self-test
scripts/demo_tc_latency.sh average
```

La prova confrontabile richiede una baseline normale a sei nodi, poi una run TC con la stessa aggregazione e gli stessi valori. Lo script riporta prima convergenza e fine della finestra di stabilità; invalida la run se osserva falsi suspect mentre tutti i container sono attivi. Dettagli in [Traffic Control](traffic_control.md).

## Artefatti e cleanup

Log, valori finali, snapshot e report sono scritti sotto `artifacts/`, escluso da Git. Gli harness tentano il cleanup anche in caso di errore; per operazioni manuali:

```bash
scripts/cluster_down.sh
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc down
```

## Limiti della valutazione

- Le topologie reali incluse sono 3 e 6 nodi; non costituiscono un benchmark di larga scala.
- Le soglie temporali Compose dipendono dalle risorse dell'host e possono richiedere override documentati nei test/script.
- Traffic Control misura uno scenario sintetico single-host, non una WAN reale.
- I grafici sono prodotti da log applicativi e non sostituiscono un sistema di tracing distribuito.
