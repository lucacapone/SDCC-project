# Configurazione

## Caricamento e formati

Il nodo accetta `--config <path>`; `.yaml`, `.yml` e `.json` sono supportati. La precedence è:

1. valori di `config.Default()`;
2. valori presenti nel file;
3. variabili environment non vuote;
4. `config.Validate`.

Il parser YAML è intenzionalmente minimale: supporta scalari `chiave: valore`, liste inline (`[a, b]`) e liste multilinea per `bootstrap_peers`, `seed_peers` ed `enabled_aggregations`. Non è un parser YAML generale: strutture annidate, anchor e tipi complessi non sono accettati. Le chiavi di logging con punto sono scalari letterali.

## Parametri applicativi

| Campo | Default | Significato |
|---|---:|---|
| `node_id` | `node-1` | Identità logica del nodo. |
| `bind_address` | `0.0.0.0` | Indirizzo locale per il socket UDP. |
| `advertise_addr` | vuoto | Endpoint `host:porta` pubblicizzato; se vuoto deriva da bind/porta, usando loopback per bind wildcard. |
| `node_port` | `7001` | Porta UDP, intervallo 1–65535. |
| `join_endpoint` | vuoto | Endpoint `host:porta` del bootstrap HTTP opzionale. |
| `bootstrap_peers` | vuoto | Peer statici preferiti quando la lista non è vuota. |
| `seed_peers` | vuoto | Fallback statico se `bootstrap_peers` è vuota. |
| `gossip_interval_ms` | `1000` | Periodo dei round. |
| `fanout` | `2` | Numero massimo di target per round. |
| `membership_timeout_ms` | `5000` | Soglia dead; la soglia suspect deriva dalla metà. |
| `enabled_aggregations` | tutte e quattro | Whitelist di aggregazioni consentite. |
| `aggregation` | `sum` | Algoritmo attivo: `sum`, `average`, `min`, `max`. |
| `initial_value` | `0` | Contributo locale del nodo. |
| `log_level` | `info` | Livello del logger strutturato. |
| `logging.remote_merge_mode` | `significant` | Log merge: `full`, `significant` o `off`. |
| `logging.log_estimate_delta_threshold` | `0` | Delta minimo non negativo usato dalla policy di log. |

`DiscoveryPeers()` sceglie l'intera lista `bootstrap_peers` se presente, altrimenti `seed_peers`; non unisce le due liste.

## Override environment

| Variabile | Campo |
|---|---|
| `NODE_ID` | `node_id` |
| `BIND_ADDRESS` | `bind_address` |
| `ADVERTISE_ADDR` | `advertise_addr` |
| `NODE_PORT` | `node_port` |
| `JOIN_ENDPOINT` | `join_endpoint` |
| `BOOTSTRAP_PEERS` | lista CSV `bootstrap_peers` |
| `SEED_PEERS` | lista CSV `seed_peers` |
| `GOSSIP_INTERVAL_MS` | `gossip_interval_ms` |
| `FANOUT` | `fanout` |
| `MEMBERSHIP_TIMEOUT_MS` | `membership_timeout_ms` |
| `ENABLED_AGGREGATIONS` | lista CSV `enabled_aggregations` |
| `AGGREGATION` | `aggregation` |
| `INITIAL_VALUE` | `initial_value` |
| `LOG_LEVEL` | `log_level` |
| `LOGGING_REMOTE_MERGE_MODE` | `logging.remote_merge_mode` |
| `LOGGING_LOG_ESTIMATE_DELTA_THRESHOLD` | `logging.log_estimate_delta_threshold` |

Valori numerici o CSV malformati causano errore; non vengono ignorati.

Due impostazioni runtime non appartengono a `Config`:

- `OBSERVABILITY_ADDR` (default `:8080`): bind del server HTTP;
- `SDCC_GENERATION_FILE` (default `/var/lib/sdcc/generation`): storage della generation.

## Membership e vincolo temporale

Il timeout esterno viene mappato in:

```text
SuspectTimeout = max(1 ms, membership_timeout_ms / 2)
DeadTimeout    = max(SuspectTimeout + 1 ms, membership_timeout_ms)
```

La validazione stima la copertura dei peer come `ceil(peer_configurati/fanout) × gossip_interval_ms` e richiede che `SuspectTimeout` sia strettamente maggiore. Una configurazione troppo aggressiva viene quindi rifiutata prima dello startup.

## Validazione

Il caricamento fallisce se:

- identità o bind sono vuoti;
- porte, host o endpoint `host:porta` non sono validi;
- liste contengono valori vuoti, duplicati o endpoint malformati;
- intervallo, fanout o timeout non sono positivi;
- la finestra di failure detection è incompatibile con la copertura gossip attesa;
- un'aggregazione non è tra `sum`, `average`, `min`, `max`;
- `aggregation` non appartiene a `enabled_aggregations`;
- modalità o threshold di logging non sono validi.

## Configurazioni incluse

- `configs/example.yaml`: esempio completo, non usato dal Compose canonico;
- `configs/node1.yaml` … `configs/node3.yaml`: cluster a 3 nodi, `average`, valori 10/30/50;
- `configs/node1.yaml` … `configs/node6.yaml`: cluster scale e TC, `average`, valori 10/30/50/70/90/110.

I file nodo usano `gossip_interval_ms: 1000`, `fanout: 5` e `membership_timeout_ms: 10000`. Con tre peer effettivi il fanout viene naturalmente limitato ai target disponibili.

## Esempi

Avvio diretto:

```bash
go run ./cmd/node --config configs/example.yaml
```

Override dell'algoritmo e del valore:

```bash
AGGREGATION=max INITIAL_VALUE=42 \
go run ./cmd/node --config configs/example.yaml
```

In un cluster reale applicare lo stesso `AGGREGATION` a tutti i nodi. Per indirizzi raggiungibili da altri container/host impostare sempre un `advertise_addr` coerente: il fallback `127.0.0.1` per bind wildcard è adatto soltanto a esecuzioni locali isolate.
