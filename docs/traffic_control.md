# Modalità sperimentale Traffic Control

## Scopo e isolamento

La modalità Traffic Control (TC) è un esperimento opzionale a sei nodi che usa
Linux `tc/netem` per ritardare **tutto il traffico in uscita** da ciascun
container. Non modifica né riusa le risorse della modalità normale: immagine,
entrypoint, Compose, rete (`sdcc-tc-net`), project name (`sdcc-tc`) e sei volumi
`*-tc-state` sono dedicati. Il binario resta quello costruito da `cmd/node` e le
configurazioni montate restano `configs/node1.yaml` … `configs/node6.yaml`.
Lo script costruisce una sola volta `sdcc-node-tc:local` dal Dockerfile TC e poi
avvia Compose con `--no-build`: tutti i sei servizi riusano quindi la stessa
immagine locale, senza build o export concorrenti sul medesimo tag.

## Avvio e profili

Dalla root, scegliere una delle aggregazioni supportate (default `average`):

```bash
scripts/demo_tc_latency.sh
scripts/demo_tc_latency.sh sum
scripts/demo_tc_latency.sh min
scripts/demo_tc_latency.sh max
```

Il comando resta responsabile dell'intera sequenza `build unica -> avvio dei sei
servizi`; non è necessario costruire manualmente l'immagine.

Lo script usa oracle statici, senza ricalcolare il risultato dei nodi:
`average=60`, `sum=360`, `min=10`, `max=110`. Compose passa la scelta tramite
l'override applicativo già supportato `AGGREGATION`; gli `initial_value` restano
`10, 30, 50, 70, 90, 110` nei YAML esistenti.

| Nodo | Delay egress | Jitter | Comportamento |
|---|---:|---:|---|
| node1 | 0 ms | 0 ms | bypass TC, nessuna qdisc NetEm |
| node2 | 500 ms | 100 ms | NetEm |
| node3 | 1000 ms | 200 ms | NetEm |
| node4 | 1500 ms | 300 ms | NetEm |
| node5 | 2000 ms | 400 ms | NetEm |
| node6 | 2500 ms | 500 ms | NetEm |

Il jitter è il 20% del delay. L'entrypoint risolve un peer, ricava dalla route la
relativa interfaccia e applica `tc qdisc replace dev <interface> root netem delay
<delay>ms <jitter>ms distribution normal`. `replace` rende l'avvio idempotente.
La risoluzione esegue un tentativo immediato e, soltanto durante il bootstrap,
fino a 40 retry ogni 250 ms (massimo 10 secondi) per assorbire la pubblicazione
concorrente dei nomi nel DNS Docker; allo scadere l'entrypoint termina non-zero.
`NET_ADMIN` è assegnata solo dai servizi TC. L'entrypoint verifica `ip`, `tc`,
route e `tc qdisc show`, fallisce chiaramente in caso di errore e usa infine
`exec`, preservando i segnali al nodo Go.

## Lettura della schermata

La demo interroga `/metrics` con `docker compose exec` su `127.0.0.1:8080`, senza
attraversare la qdisc. `known X/6` legge `sdcc_node_known_peers` e indica
**soltanto il numero di entry nella membership locale**: non indica contributi,
peer alive, contatti diretti o contributi effettivamente usati nella stima.

La stima arriva dall'ultimo `event=convergence_sample` valido, verificando anche
`node_id` e `aggregation`. `[START]` indica che manca ancora un campione; `[WAIT]`
indica una stima distante dall'oracle più di `0.000001`; `[OK]` indica
`abs(estimate-oracle) <= 0.000001`. `known` è solo informativo. La Fase A non
richiede stabilità su due controlli. La schermata si aggiorna circa ogni secondo,
termina appena i sei nodi sono contemporaneamente `[OK]` e riporta il tempo
totale; dopo 30 secondi fallisce mostrando lo stato finale.

Con `membership_timeout_ms=10000`, il runtime deriva `SuspectTimeout=5000 ms` e
`DeadTimeout=10000 ms`. La demo cerca nella run
`event=membership_transition previous_status=alive status=suspect`: se lo trova
mentre i sei container sono attivi, mostra l'evento, dichiara la run non valida e
termina non-zero, senza adattare profili o timeout.

## Diagnostica NetEm e gossip

Per verificare qdisc e contatori (sostituire il nodo quando necessario):

```bash
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc exec -T node2 sh -c \
  'dev=$(ip -o route get "$(getent ahostsv4 node1 | awk "NR==1 {print \\$1}")" | awk "{for(i=1;i<=NF;i++)if(\\$i==\"dev\"){print \\$(i+1);exit}}"); tc qdisc show dev "$dev"; tc -s qdisc show dev "$dev"'
```

Ripetendo `tc -s` durante il gossip, i contatori devono aumentare. Gli scambi
applicativi sono verificabili con:

```bash
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc logs --no-color | grep 'event=remote_merge'
```

Per un confronto non artificiale, eseguire prima la modalità normale a sei nodi
come in `docs/demo.md`, annotarne il tempo osservato e poi confrontarlo con il
`Tempo totale` della demo TC.

## Stop, cleanup e reset

`Ctrl-C` ferma il monitor lasciando il cluster disponibile per la diagnostica.
Questi comandi operano soltanto sul project TC:

```bash
# Rimuove container/rete preservando le generation nei volumi TC.
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc down

# Reset completo della sola modalità TC, generation incluse.
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc down -v --remove-orphans
```

Non usare `-v` quando si vuole mantenere l'identità durevole tra run. Le risorse
della modalità normale e del project `sdcc-scale` non vengono toccate.

## Validazione su macOS

Docker Desktop deve usare container Linux e consentire `NET_ADMIN`. Dalla root:

```bash
go test ./...
go vet ./...
bash -n deploy/traffic-control/entrypoint.sh scripts/demo_tc_latency.sh
scripts/demo_tc_latency.sh --self-test
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc config --quiet
scripts/demo_tc_latency.sh average
```

Durante la run verificare i contatori con `tc -s`, l'assenza di false suspicion
e la presenza di `remote_merge`, quindi eseguire il cleanup desiderato.

## AWS EC2 single-host

Su una EC2 Linux con Docker Engine e Compose, clonare la repository ed eseguire
gli stessi comandi. I sei container condividono il bridge dedicato e ricevono
individualmente `NET_ADMIN`; non servono nuove regole Security Group, poiché
gossip e metriche restano interni. La compatibilità effettiva dipende dal kernel:
se NetEm non è disponibile, l'entrypoint fallisce esplicitamente. A fine prova
eseguire il cleanup TC e arrestare le risorse EC2 non necessarie.

## Limiti

- Il delay è egress per container, non un RTT bidirezionale configurato.
- Il criterio Fase A usa soltanto la stima; `known` non ne aumenta la robustezza.
- Una false suspicion invalida la run invece di cambiare automaticamente profilo.
- Docker Desktop/host deve supportare realmente Linux NetEm e `NET_ADMIN`.
