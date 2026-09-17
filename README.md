# SDCC — Gossip-Based Distributed Data Aggregation

Servizio distribuito in Go che calcola aggregati globali mediante scambio gossip peer-to-peer, senza un coordinatore centrale per la computazione.

## Table of Contents

- [Overview](#overview)
- [Project Requirements](#project-requirements)
- [Features](#features)
- [Architecture](#architecture)
- [Supported Aggregations](#supported-aggregations)
- [Repository Structure](#repository-structure)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Running the Cluster](#running-the-cluster)
- [Testing](#testing)
- [Observability](#observability)
- [Deployment](#deployment)
- [Documentation](#documentation)
- [Project Report](#project-report)

## Overview

Ogni nodo mantiene una vista locale della membership e dell'aggregato, scambia periodicamente via UDP messaggi JSON contenenti stato e digest dei peer e applica merge idempotenti per contributo. I nodi sono equivalenti: seed e join endpoint servono soltanto al bootstrap, mentre gossip, failure detection e aggregazione proseguono in modo decentralizzato.

Il progetto è l'implementazione individuale di **B3 — Gossip-based distributed data aggregation** per il corso di Sistemi Distribuiti e Cloud Computing.

## Project Requirements

| Requisito B3 | Evidenza nella repository |
|---|---|
| Go e computazione gossip decentralizzata | Nodo in `cmd/node`, engine peer-to-peer in `internal/gossip` e transport UDP in `internal/transport`. |
| Almeno due aggregazioni | `sum`, `average`, `min` e `max`, selezionabili da configurazione e coperte da test dedicati. |
| Parametri configurabili | Configurazione YAML/JSON con default, override environment e validazione fail-fast. |
| Testing e robustezza ai crash | Suite unitarie, concorrenti, in-memory e Compose; scenari di crash, restart/rejoin, leave e partizione temporanea. |
| Scalabilità ed elasticità | Topologie Compose verificate a 3 e 6 nodi; join/rejoin e membership propagata via gossip. |
| Deployment AWS | Procedura principale single-host: Docker Compose su una istanza EC2 dell'AWS Academy Learner Lab. |

I risultati sperimentali non sono versionati come dataset: gli script possono produrre artefatti CSV/SVG e snapshot locali sotto `artifacts/`.

## Features

- round gossip push periodici con fanout deterministico a finestra rotante;
- payload JSON versionato e transport UDP astratto dietro interfaccia;
- deduplicazione tramite `message_id` e gestione di update duplicati, concorrenti e fuori ordine;
- membership locale con stati `alive`, `suspect`, `dead` e `leave`;
- bootstrap tramite join endpoint HTTP opzionale, con fallback ai peer statici;
- failure detection a timeout, prune e rejoin con generation/incarnation durevole;
- aggregati membership-aware: contribuiscono al risultato esposto soltanto i nodi `alive`;
- log strutturati e endpoint HTTP `/health`, `/ready`, `/metrics`;
- cluster Compose a 3 nodi, scenario scale a 6 nodi e modalità sperimentale Linux Traffic Control isolata;
- pipeline passiva per campioni di convergenza, CSV e grafico SVG.

## Architecture

Il processo carica e valida la configurazione, alloca una generation persistente, inizializza membership, observability e transport UDP, quindi avvia l'engine gossip. A ogni round applica le transizioni di failure detection, seleziona fino a `fanout` peer raggiungibili e invia lo stato completo. I merge conservano contributi e versioni per nodo, consentendo convergenza senza coordinatore.

Dettagli su protocollo, membership, versioning, merge e limiti: [Architecture](docs/architecture.md).

## Supported Aggregations

| Valore `aggregation` | Risultato |
|---|---|
| `sum` | Somma dei contributi eleggibili, con saturazione a `±math.MaxFloat64` in caso di overflow. |
| `average` | Media aritmetica ottenuta da coppie somma/conteggio per nodo. |
| `min` | Minimo dei contributi eleggibili. |
| `max` | Massimo dei contributi eleggibili. |

`aggregation` deve comparire in `enabled_aggregations`. Tutti i nodi di un cluster operativo devono usare la stessa aggregazione.

## Repository Structure

```text
.
├── cmd/                         # eseguibili node e convergence-chart
├── internal/                    # gossip, membership, transport, config e aggregazioni
├── configs/                     # esempio e configurazioni node1 ... node6
├── deploy/                      # Compose scale/TC e immagine Traffic Control
├── scripts/                     # lifecycle, risultati, demo e fault injection
├── tests/                       # suite black-box per componente e integrazione
├── docs/                        # documentazione tecnica e record operativi
├── Dockerfile                   # immagine applicativa multi-stage
├── docker-compose.yml           # cluster canonico a 3 nodi
├── Makefile                     # entry point dei test principali
└── go.mod                       # modulo Go 1.22
```

## Prerequisites

- Git;
- Go 1.22 o successivo per build e test nativi;
- Docker Engine o Docker Desktop con plugin `docker compose` per cluster e test Compose;
- Bash per gli script operativi;
- per Traffic Control: host Linux/container Linux con supporto NetEm e capability `NET_ADMIN`.

## Quick Start

Dalla root della repository:

```bash
git clone <repository-url>
cd SDCC-project
scripts/cluster_up.sh
scripts/cluster_wait_ready.sh
docker compose -p sdcc-bootstrap -f docker-compose.yml ps
docker compose -p sdcc-bootstrap -f docker-compose.yml logs --tail=30 node1
scripts/cluster_down.sh
```

`cluster_up.sh` costruisce e ricrea il cluster canonico; `cluster_wait_ready.sh` attende i servizi configurati in `deploy/compose_services.env`. Le porte di observability non sono pubblicate sull'host: gli script e i test le interrogano dall'interno dei container.

Per eliminare anche le generation persistenti:

```bash
docker compose -p sdcc-bootstrap -f docker-compose.yml down -v --remove-orphans
```

## Configuration

Il loader applica la precedence **default → file YAML/JSON → environment → validazione**. Le categorie principali sono identità e rete, discovery, intervallo/fanout gossip, timeout membership, aggregazione/valore iniziale e logging. `configs/example.yaml` mostra tutti i campi applicativi; `configs/node1.yaml` … `configs/node6.yaml` definiscono le topologie Compose.

Gli override usano nomi come `NODE_ID`, `NODE_PORT`, `SEED_PEERS`, `GOSSIP_INTERVAL_MS`, `FANOUT`, `MEMBERSHIP_TIMEOUT_MS`, `AGGREGATION` e `INITIAL_VALUE`. `OBSERVABILITY_ADDR` e `SDCC_GENERATION_FILE` sono impostazioni runtime separate.

Riferimento completo: [Configuration](docs/configuration.md).

## Running the Cluster

### Cluster canonico a 3 nodi

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f
docker compose down
```

Usa `docker-compose.yml` e `configs/node1.yaml` … `configs/node3.yaml`. L'aggregazione predefinita del cluster è `average` sui valori `10`, `30`, `50`, quindi il risultato atteso a membership stabile è `30`.

### Cluster scale a 6 nodi

```bash
SDCC_COMPOSE_FILE=deploy/docker-compose.scale.yml \
SDCC_PROJECT_NAME=sdcc-scale \
SDCC_SERVICES='node1 node2 node3 node4 node5 node6' \
scripts/cluster_up.sh

SDCC_COMPOSE_FILE=deploy/docker-compose.scale.yml \
SDCC_PROJECT_NAME=sdcc-scale \
scripts/show_aggregation_status.sh
```

I valori sono `10`, `30`, `50`, `70`, `90`, `110`; per `average` il risultato atteso è `60`. Cleanup:

```bash
SDCC_COMPOSE_FILE=deploy/docker-compose.scale.yml \
SDCC_PROJECT_NAME=sdcc-scale \
SDCC_SERVICES='node1 node2 node3 node4 node5 node6' \
scripts/cluster_down.sh
```

Il file `deploy/docker-compose.yml` è un promemoria storico e non è un entry point di deployment.

## Testing

La suite copre configurazione, factory e semantica numerica delle aggregazioni, merge gossip, fanout, membership, UDP, concorrenza, observability, generation persistente e pipeline di convergenza. I test d'integrazione includono cluster Compose, scala a sei nodi, crash/restart e uno scenario con crash sequenziali, partizione e rejoin.

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go test ./tests/integration -run 'TestClusterConvergence$' -count=1
make test-crash-restart
```

I test Compose richiedono Docker attivo e gestiscono il proprio cluster. Strategia, comandi mirati e criteri: [Testing](docs/testing.md).

## Observability

I nodi scrivono log strutturati su stdout/stderr. Un server HTTP separato, configurabile con `OBSERVABILITY_ADDR` (default `:8080`), espone:

- `/health`: liveness del processo;
- `/ready`: `503` prima dell'avvio dell'engine, poi `200`;
- `/metrics`: contatori e gauge in formato testuale Prometheus.

Sono disponibili metriche per round, merge remoti, peer noti, stima, uptime, readiness e stato lifecycle. Riferimento: [Observability](docs/observability.md).

## Deployment

### Local Docker Compose

Il percorso canonico usa il `Dockerfile` multi-stage e `docker-compose.yml` a tre nodi; `deploy/docker-compose.scale.yml` estende la stessa immagine a sei nodi. Consultare [Local Deployment](docs/deployment.md).

### AWS EC2

Il deployment B3 supportato è **una singola istanza EC2 con Docker Compose**. Gossip e metriche restano sulla rete bridge interna; dall'esterno è necessario soltanto l'accesso amministrativo alla VM. Consultare [AWS EC2 Deployment](docs/deployment_ec2.md).

## Documentation

- [Architecture](docs/architecture.md) — protocollo gossip, membership, merge e proprietà distribuite.
- [Configuration](docs/configuration.md) — schema, precedence, override e validazione.
- [Testing](docs/testing.md) — suite, scenari di robustezza e valutazione della convergenza.
- [Local Deployment](docs/deployment.md) — Dockerfile, Compose a 3/6 nodi e troubleshooting.
- [AWS EC2 Deployment](docs/deployment_ec2.md) — procedura single-host per Learner Lab.
- [Observability](docs/observability.md) — log, probe, metriche e campioni di convergenza.
- [Traffic Control](docs/traffic_control.md) — esperimento isolato Linux `tc/netem` a sei nodi.
- [Live Demo](docs/demo.md) — checklist ripetibile per presentazione e Q&A.

### Additional project records

- [AWS Learner Lab Notes](docs/aws_learner_lab_notes.md) — note operative di contesto, non documentazione canonica di deployment.
- [Operational Log](docs/operational_log.md) — cronologia append-only delle attività sul progetto.

## Project Report

Il report scientifico finale in formato ACM o IEEE double-column verrà aggiunto successivamente.
