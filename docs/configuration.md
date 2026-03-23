# Configurazione runtime canonica

Questo documento è il riferimento canonico per la configurazione del runtime del nodo.

La fonte primaria è il comportamento reale implementato in:

- `internal/config/config.go`
- `cmd/node/main.go`
- `configs/*.yaml`

## Ambito runtime

Il nodo legge la configurazione tramite il flag CLI `--config` definito in `cmd/node/main.go`. Il file viene caricato con `config.Load`, poi il risultato viene validato prima dell'avvio del runtime. Se la configurazione è invalida, il processo termina con errore. 

## Formati file supportati

Il loader runtime supporta i seguenti formati, riconosciuti **esclusivamente dall'estensione del file**:

- `.yaml`
- `.yml`
- `.json`

Qualsiasi altra estensione viene rifiutata con errore di caricamento.

### Dettagli reali del parsing YAML

Il parser YAML implementato in `internal/config/config.go` è volutamente minimale e supporta solo il sottoinsieme realmente gestito dal runtime:

- coppie top-level nel formato `chiave: valore`;
- liste inline nel formato `[a,b,c]`;
- liste multilinea solo per:
  - `bootstrap_peers`
  - `seed_peers`
  - `enabled_aggregations`

Non è un parser YAML generale: strutture annidate, mappe arbitrarie o sintassi YAML avanzata non fanno parte del contratto runtime corrente.

## Ordine di precedence effettivo

La precedence effettiva applicata dal runtime è questa:

1. **Default locali** tramite `Default()`.
2. **File di configurazione** (`.yaml/.yml` oppure `.json`) se `--config` è valorizzato.
3. **Variabili ambiente** tramite `overrideFromEnv`.
4. **Validazione finale** tramite `Validate`.

In altre parole, il flusso reale in `Load(path)` è:

```text
Default() -> parse file config -> overrideFromEnv() -> Validate()
```

### Effetti pratici della precedence

- Se `--config` è omesso, il runtime parte da `Default()` e applica solo gli eventuali override env.
- Se un file specifica un valore, quel valore sovrascrive il default.
- Se una variabile ambiente supportata è presente e valida rispetto al parser usato da `overrideFromEnv`, essa sovrascrive file e default.
- La validazione finale può comunque rifiutare il risultato anche dopo default, file ed env.

### Nota importante sugli override env

Il comportamento reale di `overrideFromEnv` è **fail-fast** per i campi numerici e CSV:

- per gli interi (`NODE_PORT`, `GOSSIP_INTERVAL_MS`, `FANOUT`, `MEMBERSHIP_TIMEOUT_MS`), se la variabile è presente ma non parseabile come intero, `Load` fallisce subito;
- per i floating-point (`INITIAL_VALUE`), se la variabile è presente ma non parseabile come numero, `Load` fallisce subito;
- per le liste CSV (`BOOTSTRAP_PEERS`, `SEED_PEERS`, `ENABLED_AGGREGATIONS`), se la variabile è presente ma contiene item vuoti o sintassi malformata, `Load` fallisce subito;
- per le stringhe, l'override continua a essere applicato solo se la variabile esiste e non è vuota dopo `TrimSpace`.

La precedence rimane invariata:

```text
Default() -> file -> env -> Validate()
```

La differenza è che un env numerico/CSV **presente e invalido** interrompe il caricamento prima della `Validate`, con un messaggio che include il nome della variabile e il valore ricevuto.

Casi espliciti fissati anche dai test di regressione:

- `NODE_PORT=abc` → errore esplicito che cita `NODE_PORT` e `abc`.
- `FANOUT=abc` → errore esplicito che cita `FANOUT` e `abc`.
- `ENABLED_AGGREGATIONS=sum,,max` → errore esplicito che cita `ENABLED_AGGREGATIONS` e il CSV ricevuto.
- `BOOTSTRAP_PEERS=node-1:7001,` → errore esplicito che cita `BOOTSTRAP_PEERS` e il CSV ricevuto.

## Elenco completo dei campi di `internal/config.Config`

La struct `Config` contiene esattamente i seguenti campi:

| Campo | Tipo | Significato operativo |
|---|---|---|
| `NodeID` | `string` | Identificativo logico del nodo. |
| `BindAddress` | `string` | Host/IP usato per il bind UDP locale. |
| `AdvertiseAddr` | `string` | Endpoint `host:port` pubblicizzato agli altri nodi; se vuoto il runtime deriva un fallback locale da `bind_address:node_port`. |
| `NodePort` | `int` | Porta UDP del nodo. |
| `JoinEndpoint` | `string` | Endpoint opzionale di join iniziale nel formato `host:porta`. |
| `BootstrapPeers` | `[]string` | Peer di bootstrap preferiti per la discovery iniziale. |
| `SeedPeers` | `[]string` | Peer seed usati come fallback se `BootstrapPeers` è vuoto. |
| `GossipIntervalMS` | `int` | Intervallo del round gossip in millisecondi. |
| `Fanout` | `int` | Parametro di fanout validato a runtime. |
| `MembershipTimeoutMS` | `int` | Timeout membership in millisecondi. |
| `EnabledAggregations` | `[]string` | Lista whitelist delle aggregazioni consentite. |
| `Aggregation` | `string` | Aggregazione attiva del nodo. |
| `InitialValue` | `float64` | Valore iniziale locale del nodo usato per seminare lo stato gossip al bootstrap. |
| `LogLevel` | `string` | Livello di log del runtime. |

## Mappatura runtime reale di `membership_timeout_ms`

Il runtime non passa più `membership_timeout_ms` come numero isolato al package membership: `cmd/node/main.go` costruisce `membership.NewSetWithConfig(cfg.MembershipConfig())`, e `internal/config.Config.MembershipConfig()` traduce il valore in una `membership.Config` concreta.

La mappatura reale e stabile è:

- `SuspectTimeout = max(1ms, membership_timeout_ms / 2)`
- `DeadTimeout = max(SuspectTimeout + 1ms, membership_timeout_ms)`

### Motivazione della regola

Questa regola preserva due proprietà operative:

- `membership_timeout_ms` resta la soglia utente che rappresenta il tempo massimo atteso prima di classificare un peer come `dead`;
- la membership mantiene sempre uno stato intermedio `suspect` osservabile, anche con valori molto piccoli, evitando che `DeadTimeout <= SuspectTimeout` venga corretto in modo implicito dal package membership.

### Esempi concreti di traduzione

| `membership_timeout_ms` | `SuspectTimeout` reale | `DeadTimeout` reale |
|---|---:|---:|
| `5000` | `2500ms` | `5000ms` |
| `3000` | `1500ms` | `3000ms` |
| `100` | `50ms` | `100ms` |
| `1` | `1ms` | `2ms` |

## Default reali di `Default()`

I default reali restituiti da `Default()` sono:

| Campo | Default |
|---|---|
| `NodeID` | `node-1` |
| `BindAddress` | `0.0.0.0` |
| `AdvertiseAddr` | `""` |
| `NodePort` | `7001` |
| `JoinEndpoint` | `""` |
| `BootstrapPeers` | `nil` |
| `SeedPeers` | `nil` |
| `GossipIntervalMS` | `1000` |
| `Fanout` | `2` |
| `MembershipTimeoutMS` | `5000` |
| `EnabledAggregations` | `['sum', 'average', 'min', 'max']` |
| `Aggregation` | `sum` |
| `LogLevel` | `info` |

## Variabili ambiente supportate da `overrideFromEnv`

Il runtime supporta esattamente queste variabili ambiente:

| Variabile ambiente | Campo target | Tipo atteso | Note reali |
|---|---|---|---|
| `NODE_ID` | `NodeID` | stringa | Ignorata se vuota/spazi. |
| `BIND_ADDRESS` | `BindAddress` | stringa | Ignorata se vuota/spazi. |
| `ADVERTISE_ADDR` | `AdvertiseAddr` | stringa | Endpoint `host:porta` pubblicizzato agli altri nodi. |
| `NODE_PORT` | `NodePort` | intero | Se presente ma non numerica fallisce il load. |
| `JOIN_ENDPOINT` | `JoinEndpoint` | stringa | Ignorata se vuota/spazi. |
| `BOOTSTRAP_PEERS` | `BootstrapPeers` | CSV | Interpretata come lista `a,b,c`; se presente ma malformata fallisce il load. |
| `SEED_PEERS` | `SeedPeers` | CSV | Interpretata come lista `a,b,c`; se presente ma malformata fallisce il load. |
| `GOSSIP_INTERVAL_MS` | `GossipIntervalMS` | intero | Se presente ma non numerica fallisce il load. |
| `FANOUT` | `Fanout` | intero | Se presente ma non numerica fallisce il load. |
| `MEMBERSHIP_TIMEOUT_MS` | `MembershipTimeoutMS` | intero | Se presente ma non numerica fallisce il load. |
| `ENABLED_AGGREGATIONS` | `EnabledAggregations` | CSV | Interpretata come lista `a,b,c`; se presente ma malformata fallisce il load. |
| `AGGREGATION` | `Aggregation` | stringa | Ignorata se vuota/spazi. |
| `LOG_LEVEL` | `LogLevel` | stringa | Ignorata se vuota/spazi. |

## Regole di validazione applicate da `Validate`

La validazione finale applica le seguenti regole.

### 1. Regole sui campi obbligatori e numerici

- `node_id` deve essere non vuoto dopo trim.
- `node_port` deve essere compreso tra `1` e `65535`.
- `gossip_interval_ms` deve essere `> 0`.
- `fanout` deve essere `> 0`.
- `membership_timeout_ms` deve essere `> 0`.
- il valore viene poi tradotto in `SuspectTimeout` e `DeadTimeout` tramite la regola runtime documentata sopra.
- `aggregation` deve essere non vuota dopo trim.
- `enabled_aggregations` deve contenere almeno un valore.

### 2. Regole su `bind_address`

- `bind_address` è obbligatorio.
- Il valore deve essere un host valido secondo le regole interne del package `config`.
- Il valore deve poter essere combinato con `node_port` tramite `net.JoinHostPort` e poi ri-parseato con `net.SplitHostPort`.

## 3. Regole sugli endpoint peer

Queste regole si applicano a:

- `advertise_addr` se valorizzato;
- `join_endpoint` se valorizzato;
- ogni elemento di `bootstrap_peers`;
- ogni elemento di `seed_peers`.

Ogni endpoint deve:

- rispettare il formato `host:porta`;
- avere host valido;
- avere porta numerica;
- avere porta compresa tra `1` e `65535`.

Inoltre:

- `bootstrap_peers` rifiuta valori vuoti;
- `seed_peers` rifiuta valori vuoti;
- entrambe le liste rifiutano duplicati inutili.

### 4. Regole sulle aggregazioni

Le aggregazioni supportate dal runtime sono **esattamente**:

- `sum`
- `average`
- `min`
- `max`

Le regole applicate sono:

- ogni elemento di `enabled_aggregations` deve essere non vuoto;
- `enabled_aggregations` non può contenere duplicati;
- ogni elemento di `enabled_aggregations` deve appartenere al set supportato (`sum`, `average`, `min`, `max`);
- `aggregation` deve appartenere allo stesso set supportato;
- `aggregation` deve essere presente in `enabled_aggregations`.

## Interazione runtime con bootstrap/discovery

Il metodo `DiscoveryPeers()` restituisce:

- `BootstrapPeers` se la lista è non vuota;
- altrimenti `SeedPeers`.

Nel bootstrap reale del nodo:

- `cmd/node/main.go` invoca `membership.Bootstrap(...)`;
- passa `cfg.JoinEndpoint` come endpoint di join opzionale;
- passa `cfg.DiscoveryPeers()` come peer di discovery iniziale.

Quindi la precedence di discovery è:

```text
join_endpoint (se disponibile nel flusso di bootstrap) + bootstrap_peers preferiti, altrimenti seed_peers
```

Più precisamente, a livello di configurazione peer locale:

```text
DiscoveryPeers() = bootstrap_peers se presenti, altrimenti seed_peers
```

## Esempio minimo locale con `configs/example.yaml`

### Avvio diretto con file di esempio

```bash
go run ./cmd/node --config configs/example.yaml
```

Il file `configs/example.yaml` definisce un nodo locale con:

- `node_id: node-1`
- `bind_address: 0.0.0.0`
- `advertise_addr: node1:7001`
- `node_port: 7001`
- `join_endpoint: bootstrap:9000`
- `bootstrap_peers` espliciti
- `seed_peers` di fallback
- aggregazioni abilitate `sum`, `average`, `min`, `max`
- aggregazione attiva `sum`

### Esempio minimo locale senza env override

```bash
go run ./cmd/node --config configs/example.yaml
```

### Esempio minimo locale con override env

```bash
NODE_ID=node-local-1 \
BIND_ADDRESS=127.0.0.1 \
ADVERTISE_ADDR=node-local-1:7101 \
NODE_PORT=7101 \
JOIN_ENDPOINT=bootstrap:9000 \
BOOTSTRAP_PEERS=node-1:7001,node-2:7002 \
SEED_PEERS=node-2:7002,node-3:7003 \
GOSSIP_INTERVAL_MS=500 \
FANOUT=1 \
MEMBERSHIP_TIMEOUT_MS=3000 \
ENABLED_AGGREGATIONS=sum,average,min,max \
AGGREGATION=min \
LOG_LEVEL=debug \
go run ./cmd/node --config configs/example.yaml
```

## Esempi di override env con nomi esatti

### Override di identificazione e bind

```bash
NODE_ID=node-dev-1 \
BIND_ADDRESS=127.0.0.1 \
NODE_PORT=7201 \
go run ./cmd/node --config configs/example.yaml
```

### Override bootstrap/discovery

```bash
JOIN_ENDPOINT=bootstrap:9000 \
BOOTSTRAP_PEERS=node-1:7001,node-2:7002 \
SEED_PEERS=node-2:7002,node-3:7003 \
go run ./cmd/node --config configs/example.yaml
```

### Override timing gossip e membership

```bash
GOSSIP_INTERVAL_MS=250 \
FANOUT=2 \
MEMBERSHIP_TIMEOUT_MS=4000 \
go run ./cmd/node --config configs/example.yaml
```

### Override aggregazioni e logging

```bash
ENABLED_AGGREGATIONS=sum,average,min,max \
AGGREGATION=average \
LOG_LEVEL=info \
go run ./cmd/node --config configs/example.yaml
```

## Elenco sintetico dei nomi env supportati

Per riferimento rapido, i nomi esatti supportati dal runtime sono:

- `NODE_ID`
- `BIND_ADDRESS`
- `NODE_PORT`
- `JOIN_ENDPOINT`
- `BOOTSTRAP_PEERS`
- `SEED_PEERS`
- `GOSSIP_INTERVAL_MS`
- `FANOUT`
- `MEMBERSHIP_TIMEOUT_MS`
- `ENABLED_AGGREGATIONS`
- `AGGREGATION`
- `LOG_LEVEL`
