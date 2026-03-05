# SDCC-project

Progetto SDCC per aggregazione dati distribuita con approccio **gossip decentralizzato**.

## Indice
- [Panoramica sistema gossip decentralizzato](#panoramica-sistema-gossip-decentralizzato)
- [Architettura ad alto livello](#architettura-ad-alto-livello)
- [Matrice decisionale architetturale (da confermare)](#matrice-decisionale-architetturale-da-confermare)
- [Sezione aggregazioni](#sezione-aggregazioni)
- [Configurazione esterna](#configurazione-esterna)
- [Avvio locale con Docker Compose](#avvio-locale-con-docker-compose)
- [Esecuzione test](#esecuzione-test)
- [Script/comandi standard](#scriptcomandi-standard)
- [Criteri di successo misurabili](#criteri-di-successo-misurabili)
- [Demo rapida](#demo-rapida)
- [Nota deploy EC2 essenziale](#nota-deploy-ec2-essenziale)

## Panoramica sistema gossip decentralizzato
Il sistema è pensato per nodi indipendenti che scambiano periodicamente informazioni in modalità peer-to-peer.

## Architettura ad alto livello
Ogni nodo usa configurazione esterna (YAML/JSON + variabili ambiente), costruisce una membership locale dai seed peer e avvia round gossip periodici con intervallo e fanout configurabili.

## Matrice decisionale architetturale (da confermare)

> Stato: **analisi opzioni**. Nessuna scelta definitiva è implementata in questo step.

### 1) Transport tra nodi

| Opzione | Pro | Contro | Impatto test | Impatto deploy EC2 |
|---|---|---|---|---|
| **A. HTTP+JSON** | Semplice da debuggare (`curl`, log testuali), integrazione Go standard (`net/http`), onboarding rapido team. | Overhead serializzazione/parsing JSON, latenza e payload più alti rispetto a binario. | Test integrazione facili con `httptest`; fault injection semplice (timeout/status code). | Apertura porte TCP semplice nei Security Group; troubleshooting immediato con tool standard. |
| **B. gRPC** | Contratti forti (proto), efficienza migliore (HTTP/2 + Protobuf), streaming nativo. | Maggior complessità iniziale (codegen, compatibilità versioni proto), debugging meno immediato. | Richiede harness più strutturato (server stub, compatibilità schema); ottimo per test di regressione contract-first. | Necessita hardening HTTP/2/TLS e gestione certificati; più robusto su scala ma setup più articolato. |
| **C. UDP best-effort** | Overhead minimo, latenza ridotta, adatto a gossip probabilistico. | Nessuna garanzia delivery/ordering, gestione frammentazione e perdita a carico applicazione. | Test più complessi e non deterministici; servono simulatori di packet-loss/reordering per validazione seria. | Security Group su UDP e tuning rete più sensibili; osservabilità e troubleshooting più difficili su EC2. |

### 2) Strategia gossip e fanout

| Opzione | Pro | Contro | Impatto test | Impatto deploy EC2 |
|---|---|---|---|---|
| **A. Push + fanout fisso** | Implementazione semplice, carico prevedibile per round. | Rischio diffusione lenta su cluster eterogenei, possibile ridondanza su peer già aggiornati. | Test convergenza riproducibili perché il numero di invii è stabile. | Capacity planning più semplice (traffico medio prevedibile). |
| **B. Pull + fanout fisso** | Riduce update inutili se i nodi richiedono solo quando necessario. | Richiede meccanismo request/response più articolato; latenza percepita maggiore prima sincronizzazione. | Servono test su stale-state e timeout request per evitare starvation. | Utile quando alcuni nodi EC2 hanno CPU limitata; attenzione a burst di richieste simultanee. |
| **C. Push-pull + fanout variabile** | Migliore velocità di convergenza in condizioni dinamiche; maggiore resilienza a perdita messaggi. | Algoritmo più complesso (adattamento fanout, rischio oscillazioni traffico). | Necessari test parametrici e soak test per verificare stabilità e budget rete. | Migliore adattabilità a cluster EC2 variabili, ma richiede monitoraggio metriche per non saturare banda. |

### 3) Coppie aggregazione candidate (almeno 3)

| Coppia candidata | Pro | Contro | Impatto convergenza | Impatto test | Impatto crash/rejoin |
|---|---|---|---|---|---|
| **A. Sum + Average** | Baseline intuitiva e già vicina al codice esistente; facile confronto con valore atteso. | Sensibile a double counting se il merge non è idempotente. | Buona con metadati corretti (es. conteggio campioni/versione). | Unit test diretti e integrazione semplice su 3 nodi. | Rejoin richiede evitare riapplicazione contributi storici duplicati. |
| **B. Count + Max** | Semantica semplice, `max` convergente monotono; utile per validare robustezza merge. | `count` può divergere senza deduplica eventi. | `max` converge molto rapidamente; `count` dipende da tracking identità update. | Test crash più facili su `max`, più severi su `count` con retry/perdita messaggi. | In crash prolungati `max` mantiene stabilità; `count` richiede recupero stato accurato. |
| **C. Average + Min** | Copre sia metrica centralità (`average`) sia estremo inferiore (`min`). | `average` richiede stato composto (sum,count), `min` sensibile a reset errati. | `min` converge monotonicamente se non ci sono reset; `average` necessita merge consistente. | Test devono includere join tardivo e valore outlier per validare stabilità. | Dopo restart, nodo con stato vecchio può ritardare convergenza `average` se manca anti-entropy robusta. |

### 4) Membership / discovery

| Opzione | Pro | Contro | Impatto test | Impatto deploy EC2 |
|---|---|---|---|---|
| **A. Seed statici** | Semplice e deterministico; nessun componente esterno aggiuntivo. | Scalabilità operativa limitata; update manuali a ogni cambio nodo. | Test molto riproducibili con topologia fissa. | Buono per PoC su poche istanze; meno pratico con autoscaling. |
| **B. Join endpoint** | Permette ingresso dinamico controllato da endpoint noto. | Introduce punto operativo critico (da rendere altamente disponibile). | Richiede test specifici su fallback endpoint e retry join. | Più flessibile in VPC EC2 dinamico; necessario hardening endpoint. |
| **C. Registry leggero solo discovery** | Discovery più dinamica senza coordinare l'aggregazione; buon compromesso operatività/decentralizzazione. | Dipendenza da componente esterno per discovery (consistenza TTL/heartbeat da gestire). | Test extra su lease expiry, aggiornamento membership e race di registrazione. | Ottimo per cluster EC2 elastici; richiede deploy e monitoraggio del registry. |

## Richiesta decisionale (risposta secca)

Per procedere con implementazioni architetturali definitive, indica una sola scelta per ciascun punto:

1. **Transport**: A / B / C
2. **Gossip + fanout**: A / B / C
3. **Coppia aggregazioni candidata**: A / B / C
4. **Membership/discovery**: A / B / C

Finché non ricevo questa decisione, mi fermo all'analisi comparativa senza introdurre modifiche strutturali definitive.

## Sezione aggregazioni
Aggregazioni abilitate via configurazione:
- `sum`
- `average`

La chiave `aggregation` seleziona l'aggregazione attiva nel nodo, validata contro `enabled_aggregations`.

## Configurazione esterna
File di esempio:
- `configs/example.yaml`

Parametri esterni principali:
- `gossip_interval_ms`
- `fanout`
- `node_port`
- `seed_peers`
- `membership_timeout_ms`
- `enabled_aggregations`

Esecuzione locale con file config:
```bash
go run ./cmd/node --config configs/example.yaml
```

Override via variabili ambiente (precedenza sull'YAML):
```bash
NODE_ID=node-custom \
NODE_PORT=7100 \
SEED_PEERS=node-1:7001,node-2:7002 \
GOSSIP_INTERVAL_MS=500 \
FANOUT=1 \
MEMBERSHIP_TIMEOUT_MS=3000 \
ENABLED_AGGREGATIONS=sum,average \
AGGREGATION=average \
go run ./cmd/node --config configs/example.yaml
```

## Avvio locale con Docker Compose
Compose multi-nodo:
- `deploy/docker-compose.yml`

Comandi:
```bash
docker compose -f deploy/docker-compose.yml up -d
docker compose -f deploy/docker-compose.yml ps
docker compose -f deploy/docker-compose.yml logs -f node1
docker compose -f deploy/docker-compose.yml down
```

Ogni servizio monta una config esterna dedicata:
- `configs/node1.yaml`
- `configs/node2.yaml`
- `configs/node3.yaml`

Per passare configurazioni personalizzate basta cambiare i file montati o impostare env nel servizio desiderato.

## Esecuzione test
```bash
go test ./...
```

## Script/comandi standard
È disponibile `Makefile` con target per esecuzioni riproducibili locali e Docker:

```bash
# Suite completa
make test

# Solo unit test (config + aggregate + membership)
make test-unit

# Integrazione convergenza gossip in-memory
make test-integration

# Robustezza crash/rejoin in-memory
make test-crash

# Esecuzione test completa dentro container Go
make docker-test
```

## Criteri di successo misurabili
I test introdotti in repository usano i seguenti criteri quantitativi:

1. **Convergenza gossip (3 nodi, transport in-memory)**:
   - differenza massima tra stati `< 0.05`
   - timeout massimo `2s`.
2. **Tolleranza a crash singolo**:
   - con `1` nodo down su `3`, il cluster residuo (`2/3`) converge con soglia `< 0.05`
   - timeout massimo `2s`.
3. **Restart/Rejoin opzionale**:
   - nodo riavviato rientra e il cluster torna a convergere con soglia `< 0.08`
   - timeout massimo `2s`.
4. **Validazione configurazione**:
   - parsing YAML/JSON corretto
   - errore obbligatorio su parametri non validi (`fanout <= 0`, `aggregation` non abilitata, ecc.).

## Demo rapida
```bash
# 1) Avvio cluster
docker compose -f deploy/docker-compose.yml up -d

# 2) Verifica servizi
docker compose -f deploy/docker-compose.yml ps

# 3) Arresto
docker compose -f deploy/docker-compose.yml down
```

## Nota deploy EC2 essenziale
Checklist minima:
1. aprire security group solo sulle porte necessarie tra nodi;
2. usare Docker + Compose anche su EC2 per mantenere parità con locale;
3. configurare indirizzi peer con DNS privato/VPC;
4. abilitare log centralizzati (CloudWatch o equivalente) per osservare convergenza gossip.
