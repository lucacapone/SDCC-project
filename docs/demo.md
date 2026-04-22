# Demo operativa cluster gossip SDCC

Questa guida descrive la demo operativa **reale** del cluster SDCC su Docker Compose e resta allineata alla documentazione canonica di test e deployment.

Riferimenti incrociati:
- panoramica repository e quickstart: [README.md](../README.md)
- strategia test canonica M09/M10: [docs/testing.md](testing.md)

## 1) Scopo demo

Obiettivo della demo:
- avviare un cluster gossip SDCC a **3 nodi** (`node1`, `node2`, `node3`);
- osservare la convergenza dell'aggregazione `average` nello scenario congelato dei valori iniziali `10`, `30`, `50`;
- verificare che i nodi convergano verso una stima comune attesa intorno a `30.0`.

## 2) Prerequisiti reali

Prerequisiti minimi richiesti nel repository corrente:
1. **Docker Engine** installato e attivo.
2. **Docker Compose plugin** disponibile come comando `docker compose`.
3. **Go locale installato (minimo `1.22`, coerente con `go.mod`)**.
4. Repository clonata localmente.
5. File di configurazione presenti e coerenti:
   - `configs/node1.yaml`
   - `configs/node2.yaml`
   - `configs/node3.yaml`

Verifica rapida prerequisiti:
```bash
docker --version
docker compose version
go version
```

## 3) Setup e avvio (comandi reali)

Eseguire dalla root repository:

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f node1
```

Note operative:
- il file Compose canonico è `docker-compose.yml` in root;
- i nodi usano i file `configs/node*.yaml` montati nel container;
- per interrompere la demo: `docker compose down`.

## 4) Cosa osservare durante la demo

Durante i log e le verifiche runtime osservare almeno questi segnali:

1. **Bootstrap**
   - marker di bootstrap completato;
   - membership iniziale popolata con peer Compose raggiungibili.

2. **Round gossip**
   - avanzamento dei round gossip sui nodi attivi;
   - scambio periodico di stato/membership tra peer.

3. **Convergenza stime**
   - le stime locali dei nodi devono avvicinarsi nella stessa banda;
   - nello scenario `10/30/50`, il riferimento informativo atteso è circa `30.0`.

4. **Readiness / metrics (dove disponibile)**
   - endpoint `/ready` coerente con bootstrap+engine avviati;
   - endpoint `/metrics` con metriche come `sdcc_node_rounds_total`, `sdcc_node_estimate`, `sdcc_node_remote_merges_total`.

## 5) Criteri di successo misurabili

I criteri devono restare coerenti con test e documentazione esistenti:

- **Convergenza M09**: banda cluster `max(values) - min(values) <= 0.05`.
- **Evidenza da log/metriche**:
  - round gossip osservabili (log e/o `sdcc_node_rounds_total` crescente);
  - nodi `ready` dopo bootstrap completo;
  - stime finali compatibili con la banda di convergenza.

Per la verifica automatica canonica:

```bash
go test ./tests/integration -run TestClusterConvergence -count=1
```

Nota di distinzione operativa:
- i comandi runtime cluster (`docker compose up/ps/logs/down`) orchestrano il deployment locale dei container;
- i comandi test canonici (`go test ./tests/integration ...`) eseguono test Go locali e richiedono la toolchain Go installata sull'host.

## 6) Scenario crash/restart (supportato)

Lo scenario crash/restart è **supportato** nel repository, ma va riferito ai flussi reali già esistenti:

- test canonico: `tests/integration/TestNodeCrashAndRestart`;
- variante rapida: `tests/integration/TestNodeCrashAndRestartInMemory`;
- script di supporto manuale: `scripts/fault_injection/*`.

Comandi di riferimento già supportati:

```bash
go test ./tests/integration -run TestNodeCrashAndRestart -count=1
scripts/fault_injection/node_stop_start.sh stop node1
scripts/fault_injection/node_stop_start.sh start node1
scripts/fault_injection/collect_debug_snapshot.sh node1
```

Questa sezione non introduce workflow nuovi: riusa esclusivamente test/script già presenti.


## 6) Scenario resilienza esteso (crash sequenziale + partizione + rejoin)

Oltre al test M10 base, la demo supporta uno scenario combinato più severo:

- crash di `node1`, poi crash di `node2`;
- partizione temporanea di rete su `node3`;
- recovery con restart e rejoin di `node1`/`node2`.

Comandi di riferimento:

```bash
go test ./tests/integration -run TestSequentialCrashPartitionAndRejoin -count=1
scripts/fault_injection/scenario_sequential_crash_partition_rejoin.sh
```

Timeout configurabili per ambienti lenti/CI:

```bash
SDCC_M10_EXT_SCENARIO_TIMEOUT=150s \
SDCC_M10_EXT_RESIDUAL_TIMEOUT=30s \
SDCC_M10_EXT_REJOIN_TIMEOUT=45s \
go test ./tests/integration -run TestSequentialCrashPartitionAndRejoin -count=1
```

Criteri di successo osservabili:

- cluster residuo operativo durante fault (nodo superstite `ready` e con round in avanzamento);
- riconvergenza cluster entro banda `0.08`;
- membership corretta dopo reintegro (`sdcc_node_known_peers >= 2` per nodo).

## 7) Troubleshooting minimo

### A) Container in stato `exited`
- Verificare stato servizi: `docker compose ps`.
- Ispezionare log del servizio: `docker compose logs --tail 200 node1` (o nodo interessato).
- Se necessario: `docker compose down && docker compose up -d --build`.

### B) Peer non raggiungibili
- Verificare che i peer nei file config usino gli hostname Compose (`node1`, `node2`, `node3`).
- Controllare rete e nomi servizi nel `docker-compose.yml` canonico.

### C) Mismatch configurazione
- Verificare coerenza tra `node_id`, `advertise_addr`, `seed_peers`, `aggregation`, `initial_value` nei file `configs/node*.yaml`.
- Evitare override env non coerenti con i file montati.

---

Per contestualizzare la demo rispetto ai target di test e ai criteri di successo, fare sempre riferimento anche a [README.md](../README.md) e [docs/testing.md](testing.md).
