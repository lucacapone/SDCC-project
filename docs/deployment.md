# Deployment locale multi-nodo con Docker Compose

## Scopo e stato canonico
Questo documento è il riferimento canonico per il deployment locale multi-nodo del progetto SDCC tramite Docker Compose.

Il file Compose operativo da usare è **sempre** quello nella root della repository:

- `docker-compose.yml`

Il file `deploy/docker-compose.yml` **non** è la sorgente operativa del deployment: è mantenuto solo come promemoria storico e rimanda esplicitamente al file Compose canonico di root.

## Artefatti allineati
Il deployment locale descritto in questo documento è allineato con i seguenti artefatti del repository:

- `README.md`, che riporta i comandi operativi standard per l'avvio locale;
- `docker-compose.yml`, che definisce i tre servizi `node1`, `node2`, `node3` e la rete bridge `sdcc-net`;
- `configs/node1.yaml`, `configs/node2.yaml`, `configs/node3.yaml`, montati in sola lettura nei container come `/config/config.yaml`.

Ogni servizio usa la stessa immagine locale `sdcc-node:local`, costruita dal `Dockerfile` di root, e passa il file di configurazione tramite il comando applicativo `--config /config/config.yaml`.

## Prerequisiti
Per eseguire il cluster locale servono:

1. **Docker Engine** installato e avviato;
2. **Docker Compose plugin** disponibile come sottocomando `docker compose`;
3. porte UDP locali non occupate per i tre nodi del cluster.

### Porte usate
Nel deployment corrente i nodi usano queste porte applicative:

- `7001/udp` per `node1`;
- `7002/udp` per `node2`;
- `7003/udp` per `node3`.

Queste porte sono dichiarate nei file `configs/node*.yaml` tramite `node_port` e vengono anche ribadite dagli override ambiente definiti nel `docker-compose.yml`.

## Topologia del cluster locale
Il cluster Compose reale prevede tre servizi:

- `node1`
- `node2`
- `node3`

La configurazione corrente è coerente tra Compose e file YAML:

- `node1` usa `node_port: 7001` e seed `node2:7002`, `node3:7003`;
- `node2` usa `node_port: 7002` e seed `node1:7001`, `node3:7003`;
- `node3` usa `node_port: 7003` e seed `node1:7001`, `node2:7002`.

Le aggregazioni configurate sono:

- `node1`: `sum`;
- `node2`: `sum`;
- `node3`: `average`.

## Comando standard di build e avvio
Dalla root della repository eseguire:

```bash
docker compose up -d --build
```

Questo comando:

- costruisce l'immagine applicativa locale a partire dal `Dockerfile`;
- crea, o aggiorna, i container `node1`, `node2`, `node3`;
- collega i servizi alla rete bridge `sdcc-net`;
- monta per ogni nodo il rispettivo file `configs/node*.yaml` in `/config/config.yaml`.

## Verifica dello stato dei servizi
Per controllare lo stato dei container avviati:

```bash
docker compose ps
```

L'output atteso deve mostrare i tre servizi Compose del cluster in stato attivo. Se un container è in restart loop o exited, consultare subito i log del servizio interessato.

## Consultazione dei log
Per seguire i log di un singolo nodo, ad esempio `node1`:

```bash
docker compose logs -f node1
```

Comandi analoghi possono essere usati per `node2` e `node3`:

```bash
docker compose logs -f node2
docker compose logs -f node3
```

I log sono il primo strumento diagnostico per verificare:

- bootstrap iniziale della membership;
- eventuali errori di bind porta;
- errori di risoluzione DNS dei peer Compose;
- mancata convergenza gossip o round non avviati.

## Stop e cleanup
Per fermare il cluster e rimuovere i container creati dal Compose:

```bash
docker compose down
```

Questo comando arresta i container del progetto Compose e rimuove le risorse create dal deployment locale, mantenendo però l'immagine locale costruita, salvo rimozione esplicita separata.

## Rete Compose e risoluzione DNS tramite service name
Il `docker-compose.yml` canonico definisce una rete bridge dedicata:

- `sdcc-net`

All'interno di questa rete, Docker Compose fornisce la risoluzione DNS automatica dei **service name**. Nel deployment corrente significa che:

- il servizio `node1` può raggiungere `node2` e `node3` usando gli hostname `node2` e `node3`;
- il servizio `node2` può raggiungere `node1` e `node3` usando gli hostname `node1` e `node3`;
- il servizio `node3` può raggiungere `node1` e `node2` usando gli hostname `node1` e `node2`.

Per questo motivo i file `configs/node*.yaml` usano peer nel formato:

- `node1:7001`
- `node2:7002`
- `node3:7003`

È importante usare i **service name Compose** nei `seed_peers`, non hostname arbitrari o nomi container esterni alla rete. In questo repository i service name e i peer configurati sono già allineati.

## Allineamento tra Compose e configurazione runtime
Nel deployment corrente esistono due livelli coerenti di configurazione:

1. **file YAML montato** (`configs/node*.yaml`);
2. **variabili ambiente del servizio Compose**.

Il runtime del progetto supporta override via environment. Per questo motivo bisogna mantenere coerenti i valori duplicati tra YAML e Compose, in particolare:

- `NODE_ID` ↔ `node_id`;
- `NODE_PORT` ↔ `node_port`;
- `SEED_PEERS` ↔ `seed_peers`;
- `GOSSIP_INTERVAL_MS` ↔ `gossip_interval_ms`;
- `FANOUT` ↔ `fanout`;
- `MEMBERSHIP_TIMEOUT_MS` ↔ `membership_timeout_ms`;
- `ENABLED_AGGREGATIONS` ↔ `enabled_aggregations`;
- `AGGREGATION` ↔ `aggregation`.

In caso di disallineamento, l'override ambiente può prevalere sul contenuto del file montato, generando diagnosi fuorvianti se si osserva solo lo YAML.

## Troubleshooting

### 1. Bootstrap race condition
**Sintomo:** uno o più nodi partono correttamente, ma all'inizio non vedono tutti i peer oppure i primi round gossip mostrano membership parziale.

**Causa probabile:** i container sono stati creati quasi simultaneamente e uno dei nodi ha tentato il contatto verso peer non ancora pronti ad accettare traffico UDP o non ancora completamente inizializzati.

**Azioni consigliate:**

- attendere alcuni round gossip e ricontrollare i log con `docker compose logs -f node1` o sul nodo interessato;
- verificare che tutti i servizi risultino attivi con `docker compose ps`;
- se la situazione non converge, rieseguire un riavvio pulito con:

```bash
docker compose down
docker compose up -d --build
```

### 2. DNS o nomi container non risolti
**Sintomo:** nei log compaiono errori verso peer non raggiungibili o hostname non risolti.

**Cause probabili:**

- i `seed_peers` non usano i service name Compose reali;
- si sta usando un file Compose diverso da quello canonico in root;
- il container non è collegato alla rete `sdcc-net` prevista.

**Azioni consigliate:**

- usare il file `docker-compose.yml` della root;
- verificare che i peer siano esattamente `node1`, `node2`, `node3` con le rispettive porte;
- controllare che i file `configs/node*.yaml` e le variabili `SEED_PEERS` del Compose coincidano.

### 3. Mismatch porte/configurazione
**Sintomo:** i container risultano avviati, ma la comunicazione tra nodi non avanza o si osservano errori di bind/invio verso porte sbagliate.

**Cause probabili:**

- `node_port` nel file YAML differente da `NODE_PORT` nell'environment del servizio;
- `seed_peers` configurati con porte diverse da quelle realmente usate dai peer;
- modifica parziale di un solo nodo senza aggiornare tutti i riferimenti incrociati.

**Azioni consigliate:**

- confrontare `docker-compose.yml` con `configs/node1.yaml`, `configs/node2.yaml`, `configs/node3.yaml`;
- mantenere allineati i valori duplicati tra YAML e variabili ambiente;
- dopo correzioni, ricreare i container con `docker compose up -d --build`.

### 4. Differenze tra ambienti Docker locali
**Sintomo:** il deployment funziona su una macchina ma non su un'altra, oppure mostra comportamenti diversi tra Docker Desktop, Engine Linux nativo o ambienti virtualizzati.

**Cause probabili:**

- differenze di networking locale o firewall host;
- plugin Compose non aggiornato o comportamento diverso della distribuzione Docker;
- risorse macchina limitate durante build o start simultaneo dei container.

**Azioni consigliate:**

- verificare che `docker compose` sia disponibile e aggiornato nell'ambiente locale;
- assicurarsi che Docker Engine sia in esecuzione stabile;
- ripetere il test con `docker compose down` seguito da `docker compose up -d --build`;
- usare i log dei nodi per distinguere problemi di applicazione da problemi del runtime Docker.

### 5. Container avviati ma membership non convergente
**Sintomo:** tutti i container sono in esecuzione, ma la membership rimane incompleta oppure il gossip non converge come atteso.

**Cause probabili:**

- bootstrap iniziale incompleto non recuperato nei round successivi;
- peer list incoerente tra YAML ed environment;
- timeout o intervalli gossip configurati in modo incoerente rispetto al contesto locale;
- modifica manuale di aggregazione o peer senza riallineamento documentale/configurativo.

**Azioni consigliate:**

- controllare i log dei tre nodi per verificare round gossip e merge membership;
- ricontrollare `gossip_interval_ms`, `membership_timeout_ms`, `seed_peers`, `aggregation` e `enabled_aggregations`;
- assicurarsi che i peer configurati siano risolvibili via DNS Compose;
- rifare un avvio pulito del cluster dopo ogni modifica strutturale alla configurazione.

## Procedura operativa consigliata
Sequenza minima consigliata per un ciclo standard di verifica locale:

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f node1
docker compose down
```

## Riferimenti correlati
- `README.md`
- `docker-compose.yml`
- `deploy/docker-compose.yml`
- `configs/node1.yaml`
- `configs/node2.yaml`
- `configs/node3.yaml`
- `docs/configuration.md`
