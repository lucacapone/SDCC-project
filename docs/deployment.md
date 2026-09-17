# Deployment locale con Docker Compose

## Artefatti

- `Dockerfile`: build Go 1.22 con `CGO_ENABLED=0`, runtime distroless non-root;
- `docker-compose.yml`: deployment canonico a 3 nodi;
- `deploy/docker-compose.scale.yml`: deployment a 6 nodi;
- `deploy/docker-compose.yml`: promemoria storico, non deployment eseguibile;
- `configs/node1.yaml` … `configs/node6.yaml`: configurazioni montate read-only;
- `scripts/cluster_*.sh`: orchestrazione condivisa da operatori e test.

Ogni servizio monta un volume distinto su `/var/lib/sdcc` per preservare la generation tra restart. Tutti i nodi condividono la rete bridge `sdcc-net` e risolvono i peer tramite i nomi DNS dei servizi Compose.

## Prerequisiti

- Docker Engine/Desktop attivo;
- plugin Docker Compose v2 (`docker compose version`);
- Bash per gli script.

Le porte UDP e HTTP sono interne alla rete Compose. I file forniti non pubblicano porte sull'host, evitando collisioni fra nodi che usano tutti `:8080` internamente per observability.

## Cluster canonico a 3 nodi

### Avvio diretto

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f
```

Il Compose root usa `node1`, `node2`, `node3`; le configurazioni selezionano `average` con valori `10`, `30`, `50`. Il risultato stabile atteso è `30`.

### Avvio tramite script

```bash
scripts/cluster_up.sh
scripts/cluster_wait_ready.sh
```

Gli script usano project name `sdcc-bootstrap`, `docker-compose.yml` e i servizi di `deploy/compose_services.env`. `cluster_up.sh` pulisce il project precedente, costruisce l'immagine, avvia i servizi e produce diagnostica se build/startup falliscono.

### Readiness e metriche

Gli endpoint non sono pubblicati sull'host e l'immagine distroless non contiene `curl`. `scripts/cluster_wait_ready.sh` verifica che ogni container sia running e che i log attestino bootstrap e avvio del transport; non equivale a una chiamata HTTP a `/ready`. Per una verifica di stato:

```bash
docker compose -p sdcc-bootstrap -f docker-compose.yml ps
docker compose -p sdcc-bootstrap -f docker-compose.yml logs --tail=100 node1
```

### Stop e cleanup

```bash
scripts/cluster_down.sh
```

Il comando preserva i volumi per consentire restart/rejoin coerenti. Per un reset distruttivo delle generation:

```bash
docker compose -p sdcc-bootstrap -f docker-compose.yml down -v --remove-orphans
```

Con Compose diretto:

```bash
docker compose down
docker compose down -v --remove-orphans  # reset distruttivo
```

## Cluster scale a 6 nodi

Entrambe le modalità seguenti avviano la stessa topologia a sei nodi definita da `deploy/docker-compose.scale.yml`. Il cluster usa valori `10`, `30`, `50`, `70`, `90`, `110`: l'average stabile atteso è `60`.

### Avvio diretto

```bash
docker compose -f deploy/docker-compose.scale.yml -p sdcc-scale up -d --build
```

Comandi utili associati:

```bash
docker compose -f deploy/docker-compose.scale.yml -p sdcc-scale ps
docker compose -f deploy/docker-compose.scale.yml -p sdcc-scale logs -f
```

Cleanup:

```bash
docker compose -f deploy/docker-compose.scale.yml -p sdcc-scale down
```

Questa è la modalità Docker Compose essenziale. La collocazione del file nella directory `deploy/` non rende il comando specifico per AWS: può essere usato normalmente in locale con Docker Desktop o Docker Engine.

### Avvio tramite script

```bash
export SDCC_COMPOSE_FILE=deploy/docker-compose.scale.yml
export SDCC_PROJECT_NAME=sdcc-scale
export SDCC_SERVICES='node1 node2 node3 node4 node5 node6'

scripts/cluster_up.sh
scripts/cluster_wait_ready.sh
scripts/show_aggregation_status.sh
```

La modalità tramite script aggiunge cleanup preventivo, controlli e diagnostica operativa. Cleanup con le stesse variabili esportate:

```bash
scripts/cluster_down.sh
```

## Variabili degli script

- `SDCC_COMPOSE_FILE`: file Compose relativo alla root o assoluto;
- `SDCC_PROJECT_NAME`: nome del project Compose;
- `SDCC_SERVICES`: lista separata da spazi o virgole;
- `SDCC_SERVICES_FILE`: file alternativo che definisce `SDCC_SERVICES`;

Altri timeout/percorsi sono definiti nei singoli script e nelle suite d'integrazione.

## Fault injection manuale

Dopo l'avvio del cluster canonico:

```bash
scripts/fault_injection/node_stop_start.sh stop node1
scripts/fault_injection/collect_debug_snapshot.sh node1
scripts/fault_injection/node_stop_start.sh start node1
scripts/fault_injection/network_partition.sh partition node3
```

Gli script ereditano file, project e lista servizi dalle stesse variabili. La partizione disconnette temporaneamente un container dalla rete Compose; lo scenario combinato è disponibile in `scenario_sequential_crash_partition_rejoin.sh`.

## Pipeline risultati

```bash
scripts/cluster_collect_results.sh
scripts/cluster_convergence_report.sh
```

Gli artefatti vengono scritti in `artifacts/cluster`. Il secondo comando delimita una nuova run e produce CSV/SVG dai campioni strutturati.

## Troubleshooting

### Docker o Compose non disponibili

Verificare:

```bash
docker info
docker compose version
```

### Container in restart loop

```bash
docker compose ps
docker compose logs --tail=100 node1
```

Cause comuni: configurazione invalida, mount mancante, endpoint non valido o file generation non scrivibile.

### Peer non convergenti

Controllare che `advertise_addr` corrisponda al nome/porta del servizio, che tutti i servizi condividano `sdcc-net`, che usino la stessa aggregazione e che i timeout non siano troppo aggressivi. Cercare eventi `transport_start`, `remote_merge` e `membership_transition`.

### Risorse residue o nomi in conflitto

Usare lo stesso project name dell'avvio:

```bash
docker compose -p sdcc-bootstrap -f docker-compose.yml down --remove-orphans
```

Non eseguire contemporaneamente cluster 3 e 6 nodi con la rete esplicitamente nominata `sdcc-net`.
