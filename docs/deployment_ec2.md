# Deploy SDCC su EC2 (AWS Learner Lab)

## 1) Scopo del deploy EC2 per SDCC

Questa guida descrive **solo** il deploy dimostrativo di SDCC su EC2 in contesto **AWS Academy Learner Lab**, allineato ai comandi e agli artefatti reali del repository.

Obiettivo pratico:

- eseguire il cluster SDCC a 3 nodi (`node1`, `node2`, `node3`) su una VM EC2;
- mantenere il flusso il più vicino possibile al percorso locale canonico (`docker-compose.yml` in root);
- validare in modo osservabile bootstrap, readiness, round gossip e convergenza.

Questa non è una guida AWS generica: è un runbook operativo focalizzato sul progetto SDCC.

---

## 2) Ipotesi architetturale principale (raccomandata): **Opzione A**

### Opzione A — 1 EC2 + Docker Compose (**percorso principale**)

Percorso raccomandato per Learner Lab:

- **una sola istanza EC2 Linux**;
- repository SDCC clonata sulla VM;
- avvio del cluster con il file canonico `docker-compose.yml` di root.

Vantaggi pratici per SDCC:

- minor costo e minor rischio budget;
- setup semplice e replicabile;
- nessuna complessità di networking multi-host per la demo.

### Opzione B — multi-host EC2 (opzionale/avanzata)

Deploy con più EC2 è possibile, ma va considerato **opzionale e avanzato**:

- maggiore costo;
- gestione IP/porte più fragile;
- bootstrap/membership tra host più complesso.

**Regola operativa**: salvo esigenze specifiche approvate, usare Opzione A come percorso principale.

---

## 3) Prerequisiti

## Regione

Nel Learner Lab usare solo regioni consentite:

- `us-east-1` (preferita);
- `us-west-2`.

## Accesso e istanza

Prerequisiti minimi:

1. Istanza EC2 Linux piccola (tipicamente `t3.small` o equivalente consentito dal lab).
2. Key pair disponibile nella regione scelta:
   - in `us-east-1` in genere è disponibile `vockey`;
   - in altre regioni potrebbe essere necessario creare/gestire una key pair dedicata.
3. Security Group minimale (vedi sezione 5).
4. Accesso alla VM via SSH (se usato):

```bash
chmod 400 <key>.pem
ssh -i <key>.pem ec2-user@<EC2_PUBLIC_IP>
```

## Runtime software sulla VM

Installare/validare:

- Docker Engine;
- Docker Compose plugin (`docker compose`);
- Git.

Verifica rapida:

```bash
docker --version
docker compose version
git --version
```

---

## 4) Costi e limiti Learner Lab (vincolanti)

Punti operativi da rispettare:

- budget progetto noto: **50 USD**;
- la metrica budget può avere ritardo di **8–12 ore**;
- limite per regione: fino a **9 istanze EC2 contemporanee**;
- limite complessivo: fino a **32 vCPU**;
- EBS con tetto totale tipico **100 GB**;
- superare limiti/costi può causare disabilitazione del lab.

Conseguenza pratica per SDCC:

- mantenere la demo su **1 EC2** quando possibile;
- evitare servizi non necessari (NAT Gateway, RDS, ECS/EKS, ecc.);
- fare cleanup esplicito a fine sessione.

---

## 5) Security Group minimo per demo SDCC

Principio: aprire solo il minimo indispensabile.

## Traffico esterno (Internet -> EC2)

- `22/tcp` da IP sorgente ristretto (solo se serve SSH);
- eventuale porta observability HTTP **solo se serve demo esterna**:
  - `8080/tcp` limitata al proprio IP pubblico (non `0.0.0.0/0` se evitabile).

## Traffico interno (tra container sulla stessa EC2)

Con Opzione A, la comunicazione gossip tra nodi avviene sulla rete Docker Compose interna (`sdcc-net`) e usa le porte UDP configurate (`7001`, `7002`, `7003`) **dentro** la rete container.

Queste porte non devono necessariamente essere esposte pubblicamente nel Security Group per la demo base su singola VM.

---

## 6) Bootstrap operativo su EC2 (comandi allineati alla repo)

Eseguire sulla VM EC2.

### 6.1 Clone repository

```bash
git clone <URL_REPO_SDCC>
cd SDCC-project
```

### 6.2 Avvio cluster Compose canonico

Comando canonico repository:

```bash
docker compose up -d --build
```

Verifica servizi:

```bash
docker compose ps
```

### 6.3 Verifica log applicativi

```bash
docker compose logs --tail 120 node1
docker compose logs --tail 120 node2
docker compose logs --tail 120 node3
```

In alternativa, usare gli script repository (utile anche per diagnostica):

```bash
scripts/cluster_up.sh
scripts/cluster_wait_ready.sh
```

### 6.4 Verifica endpoint observability

Se la porta 8080 del nodo target è raggiungibile dall’host/contesto corrente:

```bash
curl -s http://127.0.0.1:8080/health
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/ready
curl -s http://127.0.0.1:8080/metrics
```

Nota: in base al mapping porte del deployment specifico, questi check possono richiedere esecuzione locale dentro la VM o adattamento endpoint.

---

## 7) Verifica demo su EC2 (passi osservabili + criteri)

Checklist osservabile consigliata:

1. `docker compose ps` mostra `node1`, `node2`, `node3` in stato attivo.
2. Nei log compaiono marker coerenti con bootstrap/round gossip.
3. `/ready` restituisce `200` quando bootstrap+engine sono completati.
4. `/metrics` espone metriche minime (`sdcc_node_rounds_total`, `sdcc_node_estimate`, `sdcc_node_remote_merges_total`, ecc.).
5. Le stime dei nodi convergono in banda stretta nello scenario `average` con valori iniziali `10/30/50`.

Criterio di convergenza allineato alla test strategy:

- banda cluster `max(values)-min(values) <= 0.05`.

Comando canonico di verifica automatica (se eseguito nel contesto adeguato):

```bash
go test ./tests/integration -run TestClusterConvergence -count=1
```

---

## 8) Stop e cleanup (obbligatorio)

A fine demo:

1. Fermare cluster SDCC sulla VM:

```bash
docker compose down
```

2. Se usati gli script repository:

```bash
scripts/cluster_down.sh
```

3. In AWS Console/CLI:

- fermare o terminare l’istanza EC2 non più necessaria;
- verificare dischi EBS residui e altre risorse lasciate attive;
- confermare che non restino istanze compute inutili.

Cleanup è obbligatorio per evitare consumo budget involontario.

---

## 9) Limiti noti e rischi pratici

1. **IP pubblico variabile**: dopo stop/start EC2 l’IPv4 pubblico può cambiare.
2. **Differenze locale vs EC2**: latenza, performance e networking possono differire dal laptop locale.
3. **Metrica budget non realtime**: il residuo mostrato può essere in ritardo (8–12 ore).
4. **Multi-host EC2 più fragile/costoso**: gestione di seed peer, porte, IP e SG è più complessa rispetto a 1 VM.
5. **Rischio risorse dimenticate**: istanze/dischi lasciati attivi consumano budget anche fuori dalla demo.

Per SDCC in Learner Lab, la scelta robusta resta: **Opzione A (1 EC2 + Docker Compose)** come percorso standard; **multi-host solo per esigenze specifiche**.
