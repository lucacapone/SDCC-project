# AWS Academy Learner Lab — Note operative per deploy SDCC su EC2

> Documento interno di supporto al progetto SDCC.
> Scopo: riassumere i vincoli e le indicazioni operative del contesto **AWS Academy Learner Lab** che sono rilevanti per deployment, test e documentazione del progetto.
> Non sostituisce la documentazione ufficiale AWS Academy, ma ne estrae i punti utili al lavoro in repository.

## Scopo nel progetto

Questo file serve come riferimento rapido per:

* documentare il deploy del sistema su **AWS EC2**;
* evitare scelte non compatibili con i limiti del Learner Lab;
* rendere espliciti i vincoli di costo, region, accesso e persistenza;
* aiutare Codex ad allineare `README.md`, `docs/deployment_ec2.md`, `docs/demo.md` e `docs/testing.md`.

## Dati di contesto

* Ambiente: **AWS Academy Learner Lab**
* Ultimo aggiornamento istruzioni sorgente: **2025-06-24**
* Natura ambiente: **sandbox long-lived**
* Budget di progetto noto: **50 USD**
* Scadenza progetto/lab da contesto corso: **11 maggio 2026**

## Implicazioni pratiche per SDCC

Per questo progetto conviene assumere come target principale:

* deploy su **EC2 Linux**
* preferenza per **istanze piccole e poco costose**
* topologia semplice e riproducibile
* uso di **Docker Compose** oppure di binari Go su poche VM
* evitare architetture AWS complesse che non sono necessarie per un sistema gossip-based di corso

Per la documentazione del progetto, la soluzione più realistica è:

* **sviluppo locale con Docker Compose**
* **deploy dimostrativo su EC2**
* attenzione a costi, IP pubblici variabili e limiti del lab

## Regioni consentite

L’accesso ai servizi è limitato a:

* `us-east-1`
* `us-west-2`

### Implicazioni

* La documentazione del progetto deve evitare di suggerire altre regioni.
* Se si indicano esempi CLI o provisioning, usare preferibilmente una di queste due regioni.
* `us-east-1` è generalmente la scelta più semplice per il lab.

## Budget e preservazione costi

Il Learner Lab richiede attenzione forte ai costi:

* il budget residuo mostrato nell’interfaccia può avere **ritardo di 8–12 ore**
* superare il budget può causare:

  * disabilitazione dell’account lab
  * perdita del lavoro e delle risorse create

### Regole operative consigliate per SDCC

* Avviare solo le istanze strettamente necessarie.
* Spegnere le istanze quando non servono.
* Terminare le risorse non più utili.
* Evitare componenti costosi non necessari al progetto:

  * NAT Gateway
  * RDS
  * cluster ECS/EKS/EMR
  * SageMaker
* Preferire un deploy essenziale basato su poche istanze EC2.

### Implicazioni per la documentazione

Nei documenti di deploy e demo è bene includere una checklist tipo:

* verificare regione corretta
* verificare numero istanze in esecuzione
* fermare le VM a fine test
* terminare risorse inutilizzate
* non lasciare servizi compute attivi tra sessioni senza motivo

## Persistenza del lab e ciclo sessioni

L’ambiente è **long-lived**:

* quando la sessione termina, dati e risorse create nell’account restano
* alla sessione successiva il lavoro può essere ritrovato

Per EC2 però ci sono effetti importanti:

* le istanze EC2 in esecuzione possono essere poste in stato **stopped** a fine sessione
* alla sessione successiva possono essere riavviate automaticamente
* dopo stop/start, l’**IPv4 pubblico può cambiare**
* la stop protection non viene preservata automaticamente

### Implicazioni per SDCC

* Non affidarsi a IP pubblici statici non gestiti.
* Preferire configurazioni che tollerino il cambio IP.
* In documentazione, separare:

  * endpoint interni/container
  * endpoint pubblici EC2
* Se si usa un cluster multi-VM, prevedere che il riavvio richieda riallineamento degli indirizzi.

## EC2 — limiti rilevanti

### Tipi di istanza supportati

Sono supportate istanze:

* nano
* micro
* small
* medium
* large

Solo:

* **On-Demand**

### Limiti di concorrenza

Per regione supportata:

* massimo **9 istanze EC2 contemporaneamente**
* massimo **32 vCPU complessive**
* tentativi di arrivare a **20 o più istanze concorrenti** possono causare disattivazione immediata dell’account

### Storage

EBS supportato con limiti:

* massimo **100 GB**
* tipi consentiti:

  * `gp2`
  * `gp3`
  * `sc1`
  * `standard`

### AMI

Consentite AMI disponibili in:

* `us-east-1`
* `us-west-2`

Consentite, in generale:

* Quick Start AMIs
* My AMIs
* Community AMIs

Non consentite:

* AWS Marketplace AMIs
* AMI che richiedono host dedicati / dedicated instance
* opzioni particolari non compatibili col lab

### Implicazioni per SDCC

Per il progetto è ragionevole documentare:

* 1 VM EC2 per demo semplice con più container
* oppure 2–3 VM piccole per demo distribuita minima
* evitare scale-out artificiale
* non proporre architetture che richiedano molte istanze

## Key pair, accesso e credenziali

### Key pair

* In `us-east-1` è disponibile il key pair **`vockey`**
* In altre regioni, `vockey` potrebbe non essere disponibile
* Se si usa un’altra regione, può essere necessario creare una nuova key pair

### Accesso Linux via SSH

Tipicamente si usa:

```bash
ssh -i <key>.pem ec2-user@<public-ip>
```

Per macOS/Linux, prima:

```bash
chmod 400 <key>.pem
```

Nel contesto del lab, se si usa `vockey`, può essere disponibile anche:

```bash
ssh -i ~/.ssh/labsuser.pem ec2-user@<public-ip>
```

### Utente tipico

* per istanze Linux Amazon Linux: `ec2-user`

### Implicazioni per SDCC

La documentazione di deploy EC2 dovrebbe:

* dichiarare esplicitamente se usa `us-east-1`
* indicare che il metodo più semplice è con `vockey`
* non assumere che una chiave custom esista già
* ricordare che il security group deve consentire SSH se si usa accesso da client esterno

## LabRole e LabInstanceProfile

Nel Learner Lab sono preconfigurati:

* ruolo IAM: **`LabRole`**
* instance profile: **`LabInstanceProfile`**

Questi sono utili soprattutto per:

* accesso a EC2 tramite **AWS Systems Manager Session Manager**
* accesso da applicazioni in esecuzione su EC2 ad altri servizi AWS, se necessario

### Implicazioni per SDCC

Per il progetto SDCC:

* non è necessario dipendere da servizi AWS avanzati
* può però essere utile documentare che:

  * `LabInstanceProfile` abilita accesso più agevole all’istanza
  * `LabRole` esiste già e può essere riusato
* se si documenta accesso via terminale browser o SSM, citare questi elementi

## Accesso da terminale nel browser

Il Learner Lab rende disponibile **AWS CloudShell**.

### Caratteristiche utili

* terminale nel browser
* AWS CLI già disponibile
* credenziali AWS già configurate
* Python 3 disponibile
* `boto3` disponibile

### Uso pratico per SDCC

CloudShell è utile per:

* verificare risorse EC2
* usare AWS CLI per inventario e troubleshooting
* recuperare informazioni rapide sull’ambiente
* evitare dipendenza dal client locale per operazioni base AWS

### Esempio CLI utile

```bash
aws ec2 describe-instances
```

## Session Manager / accesso browser a EC2

Con `LabInstanceProfile` associato a una EC2, è possibile usare accesso da browser tramite strumenti AWS compatibili con Systems Manager / Instance Connect, quando supportato dalla configurazione dell’istanza.

### Implicazioni per SDCC

Per la guida di deploy può essere utile proporre due opzioni:

1. **SSH classico**

   * più semplice da spiegare
   * adatto a demo standard

2. **Accesso browser / SSM**

   * utile quando si vuole evitare la gestione locale delle chiavi
   * dipende dalla corretta configurazione dell’istanza e del ruolo

## IAM — limitazioni utili da ricordare

L’accesso IAM nel Learner Lab è molto limitato:

* non si possono creare utenti o gruppi liberamente
* la creazione di ruoli è limitata
* i service-linked role possono essere consentiti in alcuni casi
* `LabRole` è già presente e va preferito quando serve un ruolo preesistente

### Implicazioni per SDCC

La documentazione del progetto deve:

* evitare prerequisiti che richiedano IAM avanzato
* evitare di fondare il deploy su creazione manuale di ruoli custom
* preferire l’uso dei ruoli già disponibili nel lab

## Servizi AWS rilevanti per il progetto

Per SDCC, i servizi davvero rilevanti sono soprattutto:

* **EC2**
* **EBS**
* **VPC**
* **Security Groups**
* **CloudShell**
* **Systems Manager**
* opzionalmente **CloudFormation** per riproducibilità infrastrutturale

### Servizi non prioritari per questo progetto

In generale, per questo progetto non serve basarsi su:

* RDS
* EMR
* ECS
* EKS
* SageMaker
* Elastic Beanstalk
* NAT Gateway
* servizi managed costosi o complessi

Questi servizi possono aumentare complessità e costo senza aggiungere valore ai requisiti del corso.

## CloudFormation

CloudFormation è supportato e può assumere `LabRole`.

### Possibile uso nel progetto

Può essere utile per:

* riprodurre una piccola infrastruttura di test
* creare/ricreare risorse in modo coerente
* facilitare cleanup

### Nota pratica

Per SDCC è utile solo se il repository già prevede o beneficerebbe davvero di un provisioning dichiarativo.
Se non esiste già questa direzione architetturale, non è necessario introdurla solo per M12.

## RDS, ECS, EKS, EMR, SageMaker — nota di cautela

Il Learner Lab supporta parzialmente questi servizi, ma con limiti significativi.
Per il progetto SDCC non sono il target naturale.

### Decisione raccomandata per il repository

Nei documenti del progetto:

* non proporre questi servizi come percorso standard di deploy
* menzionarli solo come non necessari o fuori scope, se utile
* mantenere il target principale su EC2

## Security group — note pratiche

Per una demo SDCC su EC2, i security group devono essere minimali e coerenti con i componenti esposti.

Tipicamente:

* porta `22/tcp` per SSH, se serve accesso remoto
* porte applicative solo se davvero necessarie
* evitare esposizione pubblica di porte interne non indispensabili

### Implicazioni per la documentazione

La guida deve:

* elencare le porte strettamente necessarie
* distinguere traffico operativo da traffico di debug
* evitare aperture “all” non motivate

## Scelte di deploy consigliate per SDCC

### Opzione A — una sola EC2 con Docker Compose

**Raccomandata per demo semplice e contenimento costi**

Vantaggi:

* più economica
* semplice da configurare
* riproducibile
* adatta a demo funzionale

Limiti:

* meno realistica come distribuzione fisica
* tutti i nodi condividono la stessa VM

### Opzione B — più EC2 piccole

**Raccomandata solo se la repo supporta bene bootstrap e configurazione distribuita su host distinti**

Vantaggi:

* più vicina a un deploy distribuito reale
* utile per mostrare membership e gossip tra host separati

Limiti:

* più costosa
* più complessa
* più fragile rispetto ai cambi di IP e alle configurazioni di rete

### Regola per la documentazione

Codex dovrebbe documentare come percorso principale quello che è:

* realmente supportato dalla repo
* più economico
* più facile da replicare in Learner Lab

## Checklist consigliata da riportare nella documentazione di deploy

### Prima del deploy

* Verificare regione: `us-east-1` o `us-west-2`
* Verificare budget residuo
* Verificare numero di istanze già attive
* Scegliere istanza compatibile e minima necessaria
* Scegliere AMI Linux compatibile
* Verificare key pair disponibile
* Configurare security group minimo

### Durante il deploy

* Annotare IP pubblico o DNS
* Annotare eventuali porte esposte
* Verificare servizi avviati
* Verificare health/readiness se presenti
* Verificare convergenza/demo con criteri osservabili

### Dopo la demo

* Salvare eventuali log o output utili
* Fermare le istanze
* Terminare risorse non più necessarie
* Verificare eventuali costi residui e risorse dimenticate

## Checklist anti-costo da riportare in README o docs/deployment_ec2.md

* Non lasciare istanze EC2 in esecuzione senza necessità
* Non creare più VM del necessario
* Non usare NAT Gateway se non strettamente indispensabile
* Non introdurre database managed o cluster non richiesti
* Non affidarsi a servizi extra per una demo che può vivere su EC2
* Ricordare che il budget mostrato può essere in ritardo
* Fare cleanup esplicito al termine delle prove

## Troubleshooting utile per il progetto

### SSH non funziona

Controllare:

* regione corretta
* key pair corretta
* permessi del file `.pem`
* porta `22` aperta nel security group
* IP pubblico aggiornato dopo riavvio

### Istanza irraggiungibile dopo nuova sessione

Possibili cause:

* istanza fermata dal lab a fine sessione
* IP pubblico cambiato
* security group o route invariati ma host cambiato indirizzo pubblico

### Budget che sembra incoerente

Possibile causa:

* metrica budget non aggiornata in tempo reale
* ritardo di 8–12 ore

### Troppe istanze / account a rischio

Controllare:

* numero complessivo di EC2 attive
* risorse indirette create da altri servizi
* vCPU totali in uso
* eventuali ambienti lasciati accesi in sessioni precedenti

## Note per Codex

Quando usi questo file per aggiornare la documentazione del progetto:

1. Non trasformare la documentazione in una guida generica AWS.
2. Mantieni il focus sul deploy del sistema SDCC.
3. Documenta solo workflow coerenti con il codice e con la repo reale.
4. Se la repo non supporta bene un deploy multi-VM, preferisci documentare una demo su singola EC2 con più nodi/container.
5. Se proponi EC2 multi-host, evidenzia i problemi pratici:

   * IP pubblici variabili
   * bootstrap/membership
   * costo superiore
   * maggiore complessità operativa
6. Inserisci sempre avvisi espliciti su:

   * budget
   * limiti di istanze
   * regioni consentite
   * necessità di cleanup

## Decisioni documentali raccomandate per M12

Per il task M12, salvo vincoli emersi dalla repo, è ragionevole che la documentazione finale:

* usi **Quickstart locale** come percorso principale
* usi **EC2** come target deploy dimostrativo
* includa una **demo replicabile**
* includa una **checklist costi e limiti Learner Lab**
* eviti dipendenze da servizi AWS non essenziali
* espliciti che il cambio IP dopo restart può richiedere aggiornamenti di configurazione o riconnessione

## Riassunto operativo

Per questo progetto, il Learner Lab va interpretato così:

* ambiente persistente ma con sessioni che terminano
* EC2 supportato e adatto al deploy
* risorse e costi da minimizzare
* regioni ristrette
* accesso possibile via SSH, CloudShell e potenzialmente SSM
* documentazione del progetto da mantenere semplice, realistica e allineata alla repo

## Fonte sintetizzata

Sintesi interna derivata dal README del **AWS Academy Learner Lab** fornito nel contesto del progetto, con estrazione e adattamento ai bisogni di deploy/documentazione di SDCC.
