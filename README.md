# SDCC-project

Progetto SDCC per aggregazione dati distribuita con approccio **gossip decentralizzato**.

## Indice
- [Panoramica sistema gossip decentralizzato](#panoramica-sistema-gossip-decentralizzato)
- [Architettura ad alto livello](#architettura-ad-alto-livello)
- [Sezione aggregazioni](#sezione-aggregazioni)
- [Configurazione esterna](#configurazione-esterna)
- [Avvio locale con Docker Compose](#avvio-locale-con-docker-compose)
- [Esecuzione test](#esecuzione-test)
- [Demo rapida](#demo-rapida)
- [Nota deploy EC2 essenziale](#nota-deploy-ec2-essenziale)

## Panoramica sistema gossip decentralizzato
Il sistema è pensato per nodi indipendenti che scambiano periodicamente informazioni in modalità peer-to-peer.

Obiettivi principali:
- evitare single point of failure;
- propagare stato e stime aggregate in round successivi;
- convergere verso un valore condiviso anche in presenza di ritardi o perdita parziale di messaggi.

## Architettura ad alto livello
### Nodi
Ogni nodo mantiene:
- stato locale (valore osservato, metadati, timestamp/round);
- membership conosciuta (peer disponibili);
- logica di merge dello stato ricevuto.

### Round gossip
A ogni round, un nodo:
1. seleziona uno o più peer;
2. invia il proprio payload;
3. riceve payload remoto;
4. applica merge/aggiornamento locale.

### Payload scambiato
Payload minimo consigliato:
- `node_id`
- `round`
- `aggregation_type`
- `value`
- `metadata` (facoltativo: versione schema, timestamp, qualità dato)

## Sezione aggregazioni
Placeholder aggregazioni previste (da confermare durante implementazione):
1. **sum**
2. **average**

Possibili estensioni future: `min`, `max`, `count`, combinazioni pesate.

## Configurazione esterna
Il repository include un file di esempio:
- `configs/example.yaml`

Esempio utilizzo (bootstrap attuale):
```bash
go run ./cmd/node --config configs/example.yaml
```

## Avvio locale con Docker Compose
File di riferimento presente nello scaffolding:
- `docker-compose.yml`

Comandi:
```bash
docker compose up -d
docker compose ps
docker compose down
```

## Esecuzione test
Comando standard Go:
```bash
go test ./...
```

## Demo rapida
Sequenza minima con i file attuali:
```bash
# 1) Verifica file di configurazione esempio
cat configs/example.yaml

# 2) Avvio stack locale (placeholder scaffolding)
docker compose up -d

# 3) Stato servizi
docker compose ps

# 4) Arresto
docker compose down
```

## Nota deploy EC2 essenziale
Checklist minima:
1. aprire security group solo sulle porte necessarie tra nodi;
2. usare Docker + Compose anche su EC2 per mantenere parità con locale;
3. configurare indirizzi peer con DNS privato/VPC;
4. abilitare log centralizzati (CloudWatch o equivalente) per osservare convergenza gossip.
