# Modalità sperimentale Traffic Control

## Obiettivo e isolamento

La modalità opzionale misura l'effetto di ritardi e jitter di rete sulla convergenza del cluster a sei nodi. Usa Linux `tc` con qdisc `netem` senza modificare il binario Go, i file applicativi o il deployment standard.

Artefatti dedicati:

- `deploy/traffic-control/Dockerfile`: immagine Alpine con nodo, `iproute2` e `curl`;
- `deploy/traffic-control/entrypoint.sh`: applicazione e verifica della qdisc;
- `deploy/docker-compose.tc.yml`: sei servizi, rete/volumi `sdcc-tc` e `NET_ADMIN`;
- `scripts/demo_tc_latency.sh`: build, startup, diagnostica e monitor.

Il project name è sempre `sdcc-tc`; rete e volumi sono separati dal cluster normale/scale.

## Prerequisiti

- Docker Engine/Desktop con container Linux e Compose v2;
- kernel/container runtime con NetEm;
- possibilità di assegnare `NET_ADMIN` ai container;
- Bash.

La capability privilegiata è presente soltanto nel Compose TC. Il deployment normale non dipende da `tc`.

## Profili di rete

| Nodo | Delay egress | Jitter | Stato |
|---|---:|---:|---|
| `node1` | 0 ms | 0 ms | bypass, nessuna qdisc NetEm |
| `node2` | 400 ms | 80 ms | NetEm |
| `node3` | 800 ms | 160 ms | NetEm |
| `node4` | 1200 ms | 240 ms | NetEm |
| `node5` | 1600 ms | 320 ms | NetEm |
| `node6` | 2000 ms | 400 ms | NetEm |

Il jitter è il 20% del delay. L'entrypoint risolve `TC_PEER_HOST` con retry limitato a 10 secondi, ricava l'interfaccia dalla route, esegue:

```text
tc qdisc replace dev <interface> root netem delay <delay>ms <jitter>ms distribution normal
```

e verifica `tc qdisc show` prima di eseguire il nodo con `exec`. I valori devono essere interi non negativi; un errore di DNS, route, capability o qdisc termina il container.

## Avvio

Validazione preliminare:

```bash
bash -n deploy/traffic-control/entrypoint.sh scripts/demo_tc_latency.sh
scripts/demo_tc_latency.sh --self-test
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc config --quiet
```

Esecuzione:

```bash
scripts/demo_tc_latency.sh average
```

Sono supportati anche:

```bash
scripts/demo_tc_latency.sh sum
scripts/demo_tc_latency.sh min
scripts/demo_tc_latency.sh max
```

Lo script costruisce una sola volta `sdcc-node-tc:local`, poi avvia i sei servizi con `--no-build --force-recreate`. `AGGREGATION` sovrascrive uniformemente i file; gli `initial_value` restano 10, 30, 50, 70, 90, 110.

Oracle usati: `average=60`, `sum=360`, `min=10`, `max=110`.

## Verifica della qdisc

Lo script verifica automaticamente che `node1` sia in bypass e che gli altri nodi abbiano NetEm. Diagnostica manuale su `node2`:

```bash
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc exec -T node2 sh -c \
  'dev=$(ip -o route get "$(getent ahostsv4 node1 | awk "NR==1 {print \\$1}")" | awk "{for(i=1;i<=NF;i++)if(\\$i==\"dev\"){print \\$(i+1);exit}}"); tc qdisc show dev "$dev"; tc -s qdisc show dev "$dev"'
```

Ripetere `tc -s` per osservare l'aumento dei contatori. Per verificare traffico applicativo:

```bash
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc logs --no-color | \
  grep 'event=remote_merge'
```

## Osservazione della convergenza

Il monitor legge `/metrics` su loopback interno per `known_peers` e l'ultimo `convergence_sample` per la stima. Stati:

- `[START]`: nessun campione disponibile;
- `[WAIT]`: distanza dall'oracle maggiore di `0.000001`;
- `[OK]`: stima entro la tolleranza.

`known X/6` descrive la dimensione della membership locale, non il numero di contributori. Il monitor conserva il primo istante in cui tutti sono `[OK]`, continua almeno fino a 8 secondi per verificare stabilità e ha timeout totale di 30 secondi. Una regressione torna `[WAIT]` senza cancellare la prima convergenza.

La run è invalidata se nei log correnti compare una transizione `alive → suspect` mentre tutti i container sono ancora attivi. Lo script non altera automaticamente timeout o profili per nascondere il problema.

## Confronto baseline e rete ritardata

1. Avviare `deploy/docker-compose.scale.yml` con la stessa aggregazione.
2. Annotare convergenza/stabilità tramite campioni o report.
3. Eseguire il cleanup del cluster scale.
4. Avviare `scripts/demo_tc_latency.sh <aggregation>`.
5. Confrontare prima convergenza e stabilità, conservando log e condizioni della VM.

Il confronto è sperimentale: non confondere il delay egress per container con un RTT WAN simmetrico.

## Uso su EC2

Su una singola EC2 Linux usare gli stessi comandi. I container condividono un bridge dedicato; non servono nuove regole Security Group. Se il kernel non offre NetEm o il runtime non applica `NET_ADMIN`, l'entrypoint fallisce esplicitamente.

## Cleanup

`Ctrl-C` arresta il monitor ma lascia i container per la diagnostica.

```bash
# preserva le generation TC
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc down

# reset completo e distruttivo della sola modalità TC
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc down -v --remove-orphans
```

## Troubleshooting e limiti

- **DNS peer non risolto:** verificare i sei servizi e i log entro il limite di bootstrap di 10 secondi.
- **Operation not permitted:** il runtime non ha applicato `NET_ADMIN`.
- **qdisc assente:** controllare supporto `sch_netem`/kernel e interfaccia risolta dalla route.
- **false suspect:** la run è non valida; conservare log, carico host e profilo anziché aumentare automaticamente i timeout.
- Il delay è egress e sintetico; la topologia resta single-host.
- Il criterio automatico usa la stima e un oracle statico, non una misura di accuratezza su stream dinamici.
- Il monitor non produce da solo il dataset definitivo del report.
