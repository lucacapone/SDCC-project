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
Ogni nodo usa configurazione esterna (YAML/JSON + variabili ambiente), costruisce una membership locale dai seed peer e avvia round gossip periodici con intervallo configurabile. Il parametro `fanout` è già in configurazione ma nel runtime corrente non è ancora applicato alla selezione peer (invio verso tutti i peer non `dead/leave`).

## Scelte architetturali confermate
- **Transport tra nodi**: UDP + payload JSON (`[]byte`) su adapter `Transport`.
- **Strategia gossip (implementata oggi)**: push verso peer attivi (`alive`/`suspect`) con payload completo stato+membership; fanout variabile pianificato ma non ancora attivo nel loop runtime.
- **Aggregazioni richieste**: `sum`, `average`, `min`, `max`.
- **Membership/discovery**: join endpoint con fallback su seed statici da configurazione.

Queste scelte sono definitive per il progetto corrente e sostituiscono la precedente matrice comparativa.

## Decisioni confermate (2026-03-05)
- **Transport**: UDP adapter concreto (`internal/transport/udp_transport.go`) con fallback esplicito a `NoopTransport` solo in caso di errore di init.
- **Strategia gossip**: round periodici push su `Transport` astratto; selezione fanout/retry non ancora implementata (presente TODO tecnico nel codice engine).
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
- **M03**: completata lato documentazione del transport astratto/concreto, confini gossip↔adapter e contratto verificato da suite dedicata.
- **M04**: completata lato repository per `sum` (merge idempotente con contributi/versioni per nodo, gestione duplicati/out-of-order, saturazione overflow e test di convergenza dedicati).
- **M05**: completata lato repository/documentazione per estensione e consolidamento `average`/`min`/`max`, regressione multi-aggregazione e verifica coerenza architetturale.

Comandi di verifica milestone:
- M03 → `go test ./internal/... -run TestTransportContract`
- M04 → `go test ./internal/aggregation/sum -run TestSumConvergence`
- M05 → test merge `average`/`min`/`max` + regressione multi-aggregazione (vedi sezione test M05).

Documento task:
- `docs/task/M01.md`
- `docs/task/M02.md`
- `docs/task/M03.md`
- `docs/task/M04.md`
- `docs/task/M05.md`

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
La configurazione segue due livelli:
- `enabled_aggregations`: insieme delle aggregazioni consentite per il nodo (whitelist runtime);
- `aggregation`: aggregazione effettivamente attiva nel nodo e usata dal gossip locale.
La validazione fallisce se `aggregation` non appartiene a `enabled_aggregations`.
Il layer comune risiede in `internal/aggregation`, con implementazioni dedicate in `sum`, `average`, `min` e `max`.
- **Stato reale `sum`**: implementazione attiva e verificata; il merge gossip usa `state.aggregation_data.sum` con contributi/versioni per nodo, è idempotente su duplicati/out-of-order e converge con test dedicato `TestSumConvergence`.
- **Stato reale `average`**: merge gossip convergente con metadati `state.aggregation_data.average` (`contributions.sum/count` + `versions` per nodo), evitando la deriva della media pairwise.
- **Stato reale `min`/`max`**: merge gossip monotono robusto con metadati opzionali `state.aggregation_data.min/max.versions` per nodo e fallback retrocompatibile su payload legacy senza metadati.
- Overflow numerico in `sum`: saturazione esplicita a `±math.MaxFloat64` con flag `overflowed` propagato nello stato gossip.

## Configurazione esterna
Documento canonico della configurazione runtime:
- `docs/configuration.md`

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
- `docker-compose.yml` (file Compose canonico alla root)

Comandi:
```bash
docker compose up -d --build
docker compose ps
docker compose logs -f node1
docker compose down
```

Ogni servizio usa la stessa immagine applicativa locale costruita dal `Dockerfile` multi-stage e monta una config esterna dedicata:
- `configs/node1.yaml`
- `configs/node2.yaml`
- `configs/node3.yaml`

Per passare configurazioni personalizzate basta cambiare i file montati o impostare env nel servizio desiderato. La build dell'immagine avviene localmente tramite `docker compose up -d --build`, senza più usare `golang:1.22` con `go run` dentro i container.

Dettagli operativi canonici di build/deploy locale multi-nodo:
- `docs/deployment.md`

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

Comando operativo M04 (verifica convergenza `sum`):
```bash
go test ./internal/aggregation/sum -run TestSumConvergence
```

Comandi operativi M05:
```bash
# average: merge convergente per contributi/versioni per nodo
go test ./internal/gossip -run TestMergeAverageContributiConvergentiPerNodo

# min/max: merge monotono robusto e compatibilità payload legacy
go test ./internal/gossip -run 'TestMergeMinMonotonoGestisceStatoVuotoELegacy|TestMergeMaxMonotonoGestisceStatoVuotoELegacy'

# regressione multi-aggregazione: sum invariata con nuove aggregazioni abilitate
go test ./internal/gossip -run TestSumRegressionConNuoveAggregazioni
```

## Script/comandi standard
È disponibile `Makefile` con target per esecuzioni riproducibili locali e Docker:

```bash
# Suite completa
make test

# Solo unit test (config + aggregation + membership)
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
   - errore obbligatorio su parametri non validi (`fanout <= 0`, `aggregation` non abilitata, peer `host:porta` malformati, `node_port` fuori range, duplicati o valori vuoti nelle liste).

## Demo rapida
```bash
# 1) Build immagine applicativa e avvio cluster
docker compose up -d --build

# 2) Verifica servizi
docker compose ps

# 3) Arresto
docker compose down
```

## Nota deploy EC2 essenziale
Checklist minima:
1. aprire security group solo sulle porte necessarie tra nodi;
2. usare Docker + Compose anche su EC2 per mantenere parità con locale;
3. configurare indirizzi peer con DNS privato/VPC;
4. abilitare log centralizzati (CloudWatch o equivalente) per osservare convergenza gossip.
