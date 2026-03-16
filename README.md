# SDCC-project

Progetto SDCC per aggregazione dati distribuita con approccio **gossip decentralizzato**.

## Indice
- [Panoramica sistema gossip decentralizzato](#panoramica-sistema-gossip-decentralizzato)
- [Architettura ad alto livello](#architettura-ad-alto-livello)
- [Scelte architetturali confermate](#scelte-architetturali-confermate)
- [Protocollo gossip (M01)](#protocollo-gossip-m01)
- [Stato avanzamento milestone](#stato-avanzamento-milestone)
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

## Scelte architetturali confermate
- **Transport tra nodi**: UDP + payload JSON (`[]byte`) su adapter `Transport`.
- **Strategia gossip**: push-pull con fanout variabile.
- **Aggregazioni richieste**: `sum`, `average`, `min`, `max`.
- **Membership/discovery**: join endpoint con fallback su seed statici da configurazione.

Queste scelte sono definitive per il progetto corrente e sostituiscono la precedente matrice comparativa.

## Decisioni confermate (2026-03-05)
- **Transport**: UDP adapter concreto (`internal/transport/udp_transport.go`) con fallback esplicito a `NoopTransport` solo in caso di errore di init.
- **Strategia gossip**: C — Push-pull con fanout variabile.
- **Aggregazioni richieste**: **sum + average + min/max**.

## Protocollo gossip (M01)
Sintesi operativa del protocollo M01:
- `GossipMessage` include i campi principali `message_id`, `origin_node`, `state_version` (con `version_epoch` + `version_counter`), `payload`, `sent_at` e `membership` (digest serializzato con `status` + `incarnation` per peer).
- Il versioning è composto da `version_epoch + version_counter`: l'epoch separa i cicli/logical reset, il counter ordina gli aggiornamenti nello stesso epoch.
- Regole principali di merge: `duplicate_message_id` (idempotenza), `out_of_order_stale` (scarto update vecchi), `same_version_different_payload` (conflitto a parità versione) e `remote_newer_version` (applicazione update più recente).
- Comando mirato di verifica: `go test ./internal/gossip -run TestMergeRules`.

Per i dettagli completi consultare l'architettura: [docs/architecture.md](docs/architecture.md).
- Test membership dedicati: `go test ./internal/gossip -run TestMergeMembershipConvergeConDuplicatiOutOfOrder`.

## Stato avanzamento milestone
- **M01**: completata (contratto messaggio gossip, versioning `epoch+counter`, merge deterministico e test di convergenza base).
- **M02**: completata a livello repository su modello membership locale + propagazione digest gossip + merge `incarnation/status` + test dedicati.

Documento task:
- `docs/task/M01.md`
- `docs/task/M02.md`

## Raccomandazione membership / discovery
Consiglio **Opzione B (join endpoint) con fallback seed statici da configurazione**.

Perché questa scelta è la più equilibrata per il progetto:
- mantiene il sistema decentralizzato per il calcolo degli aggregati;
- consente join dinamici (elasticità) senza aggiornare manualmente tutti i file di configurazione;
- resta semplice da testare in locale e su EC2 perché i seed rimangono piano B operativo.

Impatto pratico previsto:
- `join_endpoint` è già presente in configurazione come meccanismo di bootstrap opzionale;
- nel runtime corrente (`cmd/node`) viene usato `NoopJoinClient`, quindi in pratica si applica fallback su `bootstrap_peers`/`seed_peers`;
- la membership operativa resta decentralizzata e evolve via gossip peer-to-peer.

## Sezione aggregazioni
Aggregazioni abilitate via configurazione:
- `sum`
- `average`
- `min`
- `max`

La chiave `aggregation` seleziona l'aggregazione attiva nel nodo, validata contro `enabled_aggregations`.

## Configurazione esterna
File di esempio:
- `configs/example.yaml`

Parametri esterni principali:
- `join_endpoint`
- `bootstrap_peers`
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
JOIN_ENDPOINT=bootstrap:9000 \
BOOTSTRAP_PEERS=node-1:7001,node-2:7002 \
SEED_PEERS=node-1:7001,node-2:7002 \
GOSSIP_INTERVAL_MS=500 \
FANOUT=1 \
MEMBERSHIP_TIMEOUT_MS=3000 \
ENABLED_AGGREGATIONS=sum,average,min,max \
AGGREGATION=min \
go run ./cmd/node --config configs/example.yaml
```

Flusso bootstrap all'avvio:
- il nodo prova `join_endpoint` per ottenere snapshot/delta membership iniziale;
- se il join non è disponibile, usa `bootstrap_peers` (o `seed_peers` come fallback compatibile) per seed discovery locale;
- il bootstrap non è autoritativo: dopo discovery iniziale la membership evolve solo via gossip peer-to-peer.

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
- Test transport (UDP + contratto): `go test ./internal/transport -count=1`
```bash
go test ./...
```

Comandi mirati membership (M02):
```bash
go test ./internal/membership -run TestJoinLeave
go test ./internal/membership -run TestTimeoutTransitions
go test ./internal/gossip -run TestMergeMembershipConvergeConDuplicatiOutOfOrder
go test ./internal/gossip -run TestRoundSerializzaMembershipConIncarnation
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
