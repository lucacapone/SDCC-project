# Observability

## Modello

L'observability combina:

- log strutturati su stdout/stderr per eventi e diagnostica;
- collector in-process condiviso fra wiring ed engine;
- server HTTP minimale per liveness, readiness e metriche.

Non sono inclusi Prometheus, Grafana, tracing distribuito o storage centralizzato.

## Log strutturati

Il logger usa `slog` e include campi stabili quali:

- `event`, `node_id`, `runtime_instance`;
- `round`, `peers`, `estimate`, `aggregation`;
- `result` per l'esito di merge;
- `node_state` e campi diagnostici specifici.

Eventi utili:

- `node_bootstrap` e `transport_start`;
- `gossip_round`;
- `remote_merge`;
- `membership_transition`;
- `convergence_sample`;
- `shutdown`.

`logging.remote_merge_mode` controlla il dettaglio dei merge (`full`, `significant`, `off`); `logging.log_estimate_delta_threshold` riduce gli eventi non significativi in base alla variazione di stima. I campi di conflitto vengono emessi soltanto quando pertinenti.

Esempio Compose:

```bash
docker compose -p sdcc-bootstrap -f docker-compose.yml logs --no-color node1
```

## Endpoint HTTP

Il bind è `OBSERVABILITY_ADDR`, default `:8080`.

### `/health`

Restituisce sempre HTTP 200 finché il processo e il server rispondono, con JSON contenente stato `alive`, messaggio health e lifecycle corrente.

### `/ready`

Restituisce HTTP 503 fino al completamento del bootstrap e all'avvio dell'engine; poi HTTP 200 con stato `ready`. Non verifica la convergenza globale.

### `/metrics`

Restituisce testo compatibile con l'exposition format Prometheus:

- `sdcc_node_rounds_total`;
- `sdcc_node_remote_merges_total{result=...}`;
- `sdcc_node_known_peers`;
- `sdcc_node_estimate`;
- `sdcc_node_uptime_seconds`;
- `sdcc_node_ready`;
- `sdcc_node_state{state=...}`.

`known_peers` conta le entry locali di membership; non equivale al numero di contributori `alive`. `estimate` è il valore membership-aware corrente.

## Lifecycle

Gli stati esposti sono `startup`, `bootstrap_completed`, `transport_initialized`, `engine_started`, `shutdown`. Il server HTTP parte durante lo startup e viene arrestato insieme al nodo; per questo health può essere disponibile mentre ready restituisce ancora 503.

## Accesso in Docker Compose

La porta non è pubblicata sull'host. Su Linux può essere interrogata tramite IP interno:

```bash
CID=$(docker compose -p sdcc-bootstrap -f docker-compose.yml ps -q node1)
IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$CID")
curl --fail "http://${IP}:8080/health"
curl --fail "http://${IP}:8080/ready"
curl --fail "http://${IP}:8080/metrics"
```

Su Docker Desktop l'IP interno potrebbe non essere raggiungibile dall'host; usare gli harness inclusi oppure un container diagnostico sulla stessa rete. Non modificare il Compose canonico durante un test di conformità senza registrare la variante.

## Campioni e report di convergenza

`convergence_sample` è una fotografia passiva che include almeno nodo, round, aggregazione e stima. Non influenza gossip o risultato. La pipeline:

```bash
scripts/cluster_convergence_report.sh
```

raccoglie i log di una run delimitata e usa `cmd/convergence-chart` per produrre CSV e SVG in `artifacts/cluster`. Il report calcola l'oracle dai valori configurati, valida i nodi attesi e identifica una convergenza che rimanga entro tolleranza per il resto dei campioni.

`scripts/show_aggregation_status.sh` legge invece una sola volta l'ultimo campione di ogni servizio scale già attivo; non attende, non ricalcola l'aggregato e termina non-zero se mancano dati validi.

## Uso durante demo e test

```bash
go test ./tests/observability -count=1
go test ./tests/gossip -run 'TestRoundAggiornaCollector|TestConvergenceSampleEventSchema' -count=1
```

Durante la demo correlare:

1. crescita di `rounds_total`;
2. eventi `remote_merge`;
3. stime `convergence_sample` coerenti;
4. `membership_transition` durante crash/rejoin;
5. `/ready` separato dalla convergenza.

## Limiti

- endpoint senza autenticazione/TLS;
- metriche conservate soltanto in memoria;
- nessun identificatore di trace end-to-end;
- log potenzialmente voluminosi in modalità merge `full`;
- readiness attesta il runtime avviato, non quorum, membership completa o aggregato convergente.
