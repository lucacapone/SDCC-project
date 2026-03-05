# SDCC-project

Progetto SDCC per aggregazione dati distribuita con approccio **gossip decentralizzato**.

## Indice
- [Panoramica sistema gossip decentralizzato](#panoramica-sistema-gossip-decentralizzato)
- [Architettura ad alto livello](#architettura-ad-alto-livello)
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
