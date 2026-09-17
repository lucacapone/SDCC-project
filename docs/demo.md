# Checklist per la live demo

## Obiettivo

Dimostrare in modo ripetibile startup, readiness, gossip decentralizzato, aggregazione, convergenza, continuità dopo crash e rejoin. Le sezioni scale e Traffic Control sono opzionali e vanno eseguite solo se tempo e ambiente lo consentono.

## Preparazione prima della presentazione

- [ ] Docker Engine/Desktop e Compose v2 funzionano.
- [ ] La repository è sul commit da presentare e la working tree è pulita.
- [ ] `go test ./... -count=1` è stato eseguito in precedenza con esito registrato.
- [ ] Nessun project `sdcc-bootstrap`, `sdcc-scale` o `sdcc-tc` è rimasto attivo.
- [ ] Terminali e font sono leggibili; comandi lunghi sono pronti.
- [ ] È disponibile un piano di fallback con log/artefatti raccolti, senza dichiararlo una live run.

Preflight rapido:

```bash
docker info
docker compose version
scripts/cluster_down.sh
```

## Demo principale: cluster a 3 nodi

### 1. Startup e topologia

```bash
scripts/cluster_up.sh
scripts/cluster_wait_ready.sh
docker compose -p sdcc-bootstrap -f docker-compose.yml ps
```

Mostrare tre processi equivalenti, le configurazioni `configs/node1.yaml` … `node3.yaml` e l'assenza di un servizio coordinatore.

### 2. Readiness e lifecycle

Su host Linux:

```bash
CID=$(docker compose -p sdcc-bootstrap -f docker-compose.yml ps -q node1)
IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$CID")
curl --fail "http://${IP}:8080/health"
curl --fail "http://${IP}:8080/ready"
curl --fail "http://${IP}:8080/metrics" | head -30
```

Su Docker Desktop, se l'IP bridge non è raggiungibile, usare l'esito di `cluster_wait_ready.sh` e i test observability già verificati; non cambiare il deployment durante la presentazione.

Chiarire che ready significa engine avviato, non aggregato già convergente.

### 3. Gossip e aggregazione

```bash
docker compose -p sdcc-bootstrap -f docker-compose.yml logs --no-color | \
  grep -E 'event=(gossip_round|remote_merge|convergence_sample)' | tail -30
```

Evidenziare round crescenti, merge tra peer e stime. I valori 10, 30 e 50 producono `average=30` quando tutti i nodi sono `alive`.

### 4. Convergenza verificata

```bash
go test ./tests/integration -run TestClusterConvergence -count=1 -v
```

Il criterio automatico richiede banda fra stime `<= 0.05` entro il timeout della suite. Distinguere questo test da un semplice controllo di readiness.

### 5. Crash del nodo

```bash
scripts/fault_injection/node_stop_start.sh stop node1
docker compose -p sdcc-bootstrap -f docker-compose.yml ps
sleep 12
docker compose -p sdcc-bootstrap -f docker-compose.yml logs --since=20s --no-color | \
  grep -E 'event=(membership_transition|gossip_round|convergence_sample)' | tail -40
```

Mostrare che `node2` e `node3` restano attivi, continuano i round e, dopo failure detection, escludono il contributo non `alive`.

### 6. Restart e rejoin

```bash
scripts/fault_injection/node_stop_start.sh start node1
sleep 12
docker compose -p sdcc-bootstrap -f docker-compose.yml logs --since=20s --no-color | \
  grep -E 'event=(membership_transition|remote_merge|convergence_sample)' | tail -50
```

Mostrare generation/incarnation superiore, ritorno `alive` e riconvergenza a 30. Il test completo ripetibile è:

```bash
go test ./tests/integration -run TestNodeCrashAndRestart -count=1 -v
```

### 7. Cleanup principale

```bash
scripts/cluster_down.sh
```

Non usare la rimozione volumi se si vuole mostrare la persistenza della generation.

## Estensione opzionale: 6 nodi

```bash
export SDCC_COMPOSE_FILE=deploy/docker-compose.scale.yml
export SDCC_PROJECT_NAME=sdcc-scale
export SDCC_SERVICES='node1 node2 node3 node4 node5 node6'
scripts/cluster_up.sh
scripts/cluster_wait_ready.sh
scripts/show_aggregation_status.sh
```

Il risultato atteso è `average=60`. Per un grafico:

```bash
scripts/cluster_convergence_report.sh
```

Mostrare il CSV/SVG prodotto sotto `artifacts/cluster`, quindi:

```bash
scripts/cluster_down.sh
unset SDCC_COMPOSE_FILE SDCC_PROJECT_NAME SDCC_SERVICES
```

## Estensione opzionale: Traffic Control

```bash
scripts/demo_tc_latency.sh --self-test
scripts/demo_tc_latency.sh average
```

Mostrare i profili 0–2000 ms, la verifica qdisc, il primo istante di convergenza e la finestra di stabilità. Non eseguire contemporaneamente il cluster scale. Cleanup:

```bash
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc down
```

## Q&A: punti da chiarire

- Il calcolo è decentralizzato; seed/join endpoint servono solo al bootstrap.
- Fanout limita gli invii per round, ma il payload completo cresce con membership e contributi.
- UDP può perdere/riordinare messaggi; idempotenza e round successivi consentono la convergenza eventuale.
- Solo peer `alive` contribuiscono alla stima esposta; metadata non eleggibili restano disponibili per rejoin.
- Persistenza della generation rende monotono il restart della stessa identità, finché i volumi non vengono eliminati.
- Traffic Control è una valutazione sintetica single-host, non una WAN reale.
- Il report scientifico e il dataset definitivo sono deliverable successivi.

## Cleanup di emergenza

```bash
docker compose -p sdcc-bootstrap -f docker-compose.yml down --remove-orphans
docker compose -p sdcc-scale -f deploy/docker-compose.scale.yml down --remove-orphans
docker compose -p sdcc-tc -f deploy/docker-compose.tc.yml down --remove-orphans
```

Usare `-v` soltanto se si vuole eliminare intenzionalmente tutta la persistenza delle generation.
