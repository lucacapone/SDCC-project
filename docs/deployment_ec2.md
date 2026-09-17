# Deployment su AWS EC2

## Ambito

Il percorso supportato per B3 è **una singola istanza EC2 dell'AWS Academy Learner Lab con Docker Compose**. Tutti i nodi sono container sulla stessa VM e comunicano sulla rete bridge; non è previsto un deployment multi-host.

Le limitazioni specifiche del laboratorio sono raccolte separatamente in [AWS Learner Lab Notes](aws_learner_lab_notes.md).

## 1. Preparare l'istanza

Creare una EC2 Linux x86_64 compatibile con Docker, dimensionata per almeno tre container (sei per gli scenari scale/TC), con volume sufficiente per immagini e log. Associare la key pair e il profilo consentiti dal Learner Lab.

Security Group minimo:

- ingresso SSH TCP/22 soltanto dal proprio indirizzo IP, oppure accesso browser/Session Manager se disponibile;
- uscita HTTPS/DNS necessaria a installazione pacchetti, clone e pull delle immagini;
- nessuna regola pubblica per UDP gossip o porta 8080: restano interne al bridge Docker.

Non esporre metriche prive di autenticazione a Internet. Se si aggiunge temporaneamente un port mapping per la demo, limitarlo al proprio IP e rimuoverlo dopo l'uso.

## 2. Installare i prerequisiti

Installare con il package manager della distribuzione:

- Git;
- Docker Engine;
- plugin Docker Compose v2.

Abilitare e avviare Docker, aggiungere l'utente al gruppo Docker se appropriato, quindi aprire una nuova sessione. Verificare:

```bash
git --version
docker info
docker compose version
```

I comandi esatti di installazione dipendono dall'AMI scelta; seguire la documentazione ufficiale della distribuzione anziché copiare comandi per un sistema diverso.

## 3. Ottenere il progetto

```bash
git clone <repository-url>
cd SDCC-project
```

## 4. Avviare il cluster a 3 nodi

```bash
scripts/cluster_up.sh
scripts/cluster_wait_ready.sh
docker compose -p sdcc-bootstrap -f docker-compose.yml ps
docker compose -p sdcc-bootstrap -f docker-compose.yml logs --tail=50
```

Il build usa il `Dockerfile` multi-stage e produce `sdcc-node:local`. I tre nodi eseguono `average` sui valori 10, 30 e 50; il risultato atteso è 30.

Per una prova automatica completa:

```bash
go test ./tests/integration -run TestClusterConvergence -count=1
go test ./tests/integration -run TestNodeCrashAndRestart -count=1
```

Questi comandi richiedono anche Go 1.22 sulla VM. Se Go non è installato, usare gli script e la checklist manuale di [Demo](demo.md).

## 5. Avviare il cluster a 6 nodi

Arrestare prima il cluster a tre nodi, perché entrambi nominano la rete `sdcc-net`:

```bash
scripts/cluster_down.sh

export SDCC_COMPOSE_FILE=deploy/docker-compose.scale.yml
export SDCC_PROJECT_NAME=sdcc-scale
export SDCC_SERVICES='node1 node2 node3 node4 node5 node6'

scripts/cluster_up.sh
scripts/cluster_wait_ready.sh
scripts/show_aggregation_status.sh
```

L'average atteso per i sei valori configurati è 60. Consultare i log con:

```bash
docker compose -p sdcc-scale -f deploy/docker-compose.scale.yml logs --tail=100
```

## 6. Readiness, log e metriche

I Compose non pubblicano le porte HTTP. `cluster_wait_ready.sh` è il controllo operativo canonico. Per dimostrare `/health`, `/ready` e `/metrics` senza modificare il deployment, ricavare l'IP interno del container dalla rete Docker e interrogare la porta 8080 dalla VM, se la configurazione Docker lo consente:

```bash
NODE_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' \
  "$(docker compose -p sdcc-bootstrap -f docker-compose.yml ps -q node1)")
curl --fail "http://${NODE_IP}:8080/health"
curl --fail "http://${NODE_IP}:8080/ready"
curl --fail "http://${NODE_IP}:8080/metrics"
```

Per gli eventi applicativi:

```bash
docker compose -p sdcc-bootstrap -f docker-compose.yml logs --no-color | \
  grep -E 'event=(gossip_round|remote_merge|convergence_sample|membership_transition)'
```

## 7. Crash e rejoin

```bash
scripts/fault_injection/node_stop_start.sh stop node1
docker compose -p sdcc-bootstrap -f docker-compose.yml ps
scripts/fault_injection/node_stop_start.sh start node1
scripts/fault_injection/collect_debug_snapshot.sh node1
```

I volumi devono essere preservati tra stop/start: contengono la generation necessaria affinché il rejoin della stessa `node_id` prevalga sullo stato precedente.

## 8. Traffic Control opzionale

Su kernel Linux con NetEm disponibile:

```bash
scripts/demo_tc_latency.sh --self-test
scripts/demo_tc_latency.sh average
```

La modalità costruisce un'immagine Alpine dedicata, assegna `NET_ADMIN` soltanto ai container TC e non richiede aperture nel Security Group. Vedere [Traffic Control](traffic_control.md).

## 9. Cleanup e controllo costi

Cluster standard:

```bash
scripts/cluster_down.sh
```

Cluster scale (con le variabili ancora esportate):

```bash
scripts/cluster_down.sh
```

Traffic Control:

```bash
docker compose -f deploy/docker-compose.tc.yml -p sdcc-tc down
```

Usare `docker compose ... down -v` soltanto per un reset definitivo. Dopo la demo arrestare o terminare la EC2 secondo le regole del Learner Lab e controllare che non restino volumi/istanze inutilizzati.

## 10. Troubleshooting

- **SSH irraggiungibile:** controllare sessione Learner Lab, IP pubblico, route, key pair e regola TCP/22 limitata al proprio IP.
- **Docker permission denied:** usare una nuova sessione dopo l'aggiunta al gruppo Docker o il meccanismo amministrativo previsto dall'AMI.
- **Build lenta o spazio esaurito:** verificare `df -h`, `docker system df` e rimuovere soltanto risorse non necessarie.
- **Container in restart:** consultare `docker compose ... ps` e `logs`; verificare mount, configurazione e scrittura dei volumi.
- **Metriche non raggiungibili:** usare IP interno/porta 8080 dalla VM; non aprire automaticamente la porta a Internet.
- **NetEm fallisce:** verificare supporto kernel e capability `NET_ADMIN`; la modalità standard non dipende da Traffic Control.
