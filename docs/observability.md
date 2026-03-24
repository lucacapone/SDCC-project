# Observability minima del nodo

## Decisione canonica e vincolante
La scelta univoca del repository per l'observability minima è la **soluzione ibrida**: **stdout strutturato per gli eventi applicativi** + **endpoint HTTP dedicati per metriche, health e readiness**. Questa decisione è vincolante per i task successivi e sostituisce ogni precedente ambiguità tra opzione HTTP-only, stdout-only o implementazioni parallele.

Conseguenze operative della decisione:
- **i log evento-per-evento** devono continuare a uscire su stdout/stderr in formato strutturato tramite `log/slog`;
- **metriche e probe** devono essere esposte solo tramite il piccolo server HTTP di observability;
- **non sono ammesse implementazioni duplicate** che pubblichino le stesse metriche sia su stdout sia via endpoint alternativi non documentati;
- i done criteria relativi a metriche, liveness e readiness si considerano soddisfatti solo quando sono verificabili attraverso `/metrics`, `/health` e `/ready`, mentre stdout resta il canale canonico per il debugging sequenziale degli eventi.

## 1. Architettura minima dell'observability
L'architettura di observability introdotta e consolidata nel repository è volutamente piccola, coerente con il runtime attuale e priva di dipendenze esterne obbligatorie. I componenti minimi sono quattro:

1. **logger strutturato** basato su `log/slog`, usato per emettere eventi applicativi con chiavi stabili e leggibili sia in locale sia nei log containerizzati;
2. **collector di metriche** nel package `internal/observability`, responsabile di mantenere contatori/gauge aggregate a bassa cardinalità e di renderle disponibili via HTTP;
3. **stato lifecycle del nodo** con progressione monotona `startup -> bootstrap_completed -> transport_initialized -> engine_started -> shutdown`, utile per health/readiness e debugging operativo;
4. **server HTTP minimo** integrato nel lifecycle reale di `cmd/node/main.go`, esposto di default su `:8080` e configurabile tramite `OBSERVABILITY_ADDR`.

Flusso minimo:
- il processo avvia collector e logger durante il bootstrap del nodo;
- il lifecycle aggiorna lo stato del collector durante bootstrap, inizializzazione transport, start engine e shutdown;
- gli handler HTTP leggono lo snapshot corrente del collector ed espongono `/health`, `/ready` e `/metrics`;
- i log strutturati restano la traccia evento-per-evento, mentre le metriche offrono una vista aggregata del comportamento del nodo.

## 2. Campi log supportati
I log sono strutturati e ruotano attorno a un insieme piccolo di chiavi stabili, per evitare payload rumorosi o ad alta cardinalità. I campi supportati/attesi sono:

- `event`: nome logico dell'evento (`node_bootstrap`, `transport_start`, `gossip_round`, `remote_merge`, `shutdown`);
- `node_id`: identificatore logico del nodo che emette il log;
- `runtime_instance`: identificativo stabile dell'istanza runtime che emette il log (letto da `HOSTNAME`, con fallback a `node_id` e poi `unknown`) per distinguere immediatamente container/processi diversi anche quando condividono lo stesso `node_id`;
- `round`: round gossip locale, quando applicabile;
- `peers`: numero di peer considerati nel round corrente;
- `estimate`: stima/valore aggregato osservabile in quel momento;
- `result`: esito sintetico di un merge remoto o di un'azione significativa (`applied`, `skipped`, `conflict`, `unknown`);
- `node_state`: stato lifecycle corrente del nodo, utile soprattutto in prossimità di readiness/shutdown.

Scelte operative sui log:
- il set di campi è **stabile e intenzionalmente minimo**;
- i dettagli verbosi del payload gossip non vengono serializzati nei log ordinari;
- la cardinalità resta bassa per rendere i log leggibili e correlabili con le metriche.

Semantica esplicita per `event=remote_merge`:
- `membership_entries` indica **solo** il numero di entry ricevute nel messaggio remoto (`len(msg.membership)`);
- `peers` indica invece il numero peer **localmente noti dopo** merge stato + aggiornamento membership (`len(Membership.Snapshot())`);
- i due campi devono rimanere separati e non intercambiabili, così da distinguere chiaramente ampiezza del payload remoto e vista locale corrente del nodo.

## 3. Metriche esposte
L'endpoint `/metrics` espone un formato testuale minimale pensato per verifica umana, scraping semplice e test automatici mirati. Le metriche/documenti di stato esposti sono:

- **round gossip**: conteggio aggregato dei round eseguiti dal nodo;
- **merge remoti**: conteggio dei merge remoti per esito, con cardinalità limitata ai valori `applied`, `skipped`, `conflict`, `unknown`;
- **readiness del nodo**: stato booleano/derivato che riflette se bootstrap ed engine risultano effettivamente completati;
- **stato lifecycle del nodo**: gauge `sdcc_node_state{state=...}` che rende osservabile la fase corrente del nodo;
- **health applicativa minima**: esposta indirettamente tramite gli handler HTTP e coerente con lo snapshot lifecycle corrente.

Principi adottati:
- niente etichette per peer, message ID o endpoint remoti, per evitare esplosione di cardinalità;
- metriche centrate sul comportamento del nodo, non su tracing distribuito fine-grained;
- naming e contenuto mantenuti abbastanza stabili da poter essere validati dal test canonico `TestMetricsExposure`.

## 4. Endpoint disponibili
Gli endpoint HTTP disponibili sono tre.

### `/health`
- scopo: **liveness** minima del processo;
- comportamento: restituisce `200 OK` finché il processo HTTP/runtime è vivo;
- contenuto utile: include il `node_state` corrente per favorire il debugging rapido.

### `/ready`
- scopo: **readiness** del nodo per uso locale/Compose/debug;
- comportamento: restituisce `503` finché il nodo non ha completato bootstrap e avvio engine;
- transizione a pronto: restituisce `200 OK` quando il nodo raggiunge `engine_started`;
- **criterio canonico di readiness**: il nodo è pronto solo quando il collector ha già osservato sia il completamento del bootstrap sia l'avvio effettivo dell'engine gossip; il semplice fatto che il processo sia in esecuzione non è sufficiente.

### `/metrics`
- scopo: esportazione testuale delle metriche minime del nodo;
- comportamento: rende visibili contatori/gauge aggregate del collector;
- aggiornamento runtime: l'engine gossip incrementa `sdcc_node_rounds_total` dopo ogni round completato, aggiorna `sdcc_node_known_peers`/`sdcc_node_estimate` dopo round e merge remoti e registra `sdcc_node_remote_merges_total{result=...}` subito dopo `applyRemote(...)`.
- uso tipico: verifica manuale via `curl`, scraping leggero e test automatico canonico di regressione.

Binding HTTP:
- default: `:8080`;
- override: variabile ambiente `OBSERVABILITY_ADDR`.

Lifecycle del server HTTP:
- il server di observability viene avviato dal runtime del nodo durante il bootstrap di `cmd/node/main.go`;
- resta attivo per tutta la vita del processo, così da offrire una superficie stabile per `curl`, Compose e verifiche manuali;
- durante l'avvio espone subito `/health`, mentre `/ready` rimane non-pronto (`503`) finché il lifecycle non raggiunge `engine_started`;
- durante lo shutdown il processo aggiorna `node_state` a `shutdown`, quindi la disponibilità degli endpoint termina con l'arresto del processo stesso.

## 5. Istruzioni d'uso e verifica
### Avvio del nodo con observability attiva
Esempio minimale:

```bash
go run ./cmd/node --config configs/example.yaml
```

Esempio con binding HTTP esplicito:

```bash
OBSERVABILITY_ADDR=:8080 go run ./cmd/node --config configs/example.yaml
```

### Verifica canonica automatica post-M11
Il comando canonico di verifica della milestone/documentazione observability è:

```bash
go test ./tests/observability -run TestMetricsExposure
```

### Verifica manuale rapida
Con il nodo in esecuzione:

```bash
curl -s http://127.0.0.1:8080/health
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/ready
curl -s http://127.0.0.1:8080/metrics
```

Cosa aspettarsi:
- `/health` risponde positivamente finché il processo è vivo;
- `/ready` passa da non-pronto a pronto solo dopo bootstrap+start engine;
- `/metrics` include le metriche minime del collector e lo stato lifecycle del nodo.

### Correlazione pratica log + metriche
Per una diagnosi locale rapida:
1. controllare nei log strutturati la sequenza `node_bootstrap -> transport_start -> gossip_round`;
2. verificare che `node_state` avanzi fino a `engine_started`;
3. interrogare `/ready`;
4. infine verificare in `/metrics` la presenza delle metriche coerenti con il lifecycle e con eventuali merge/round già eseguiti.

## 6. Limiti noti e scelte progettuali
### Limiti noti
- l'observability è **minima**: non include tracing distribuito, profiling o integrazione nativa con backend esterni;
- il formato `/metrics` è volutamente semplice e limitato alle esigenze del repository;
- la readiness riflette il wiring del runtime attuale, non la salute end-to-end dell'intero cluster;
- le metriche sono aggregate per nodo e non descrivono in dettaglio ogni peer o ogni messaggio gossip;
- la validazione automatica copre l'esposizione minima (`TestMetricsExposure`), non un ambiente observability completo di produzione.

### Scelte progettuali
- **integrazione diretta in `cmd/node/main.go`**: non è stato introdotto un layer aggiuntivo perché il wiring richiesto resta piccolo e leggibile;
- **bassa cardinalità prima della ricchezza del dato**: priorità a metriche sostenibili e log stabili, più utili per debug e CI rispetto a un output molto dettagliato ma rumoroso;
- **soluzione ibrida fissata esplicitamente**: stdout strutturato per eventi e HTTP per metriche/probe; nessuna delle due superfici sostituisce l'altra;
- **health/readiness separate**: `/health` segnala vita del processo, `/ready` segnala la disponibilità funzionale minima del nodo;
- **command-centric verification**: la prova canonica resta il comando `go test ./tests/observability -run TestMetricsExposure`, così da avere una verifica ripetibile e non ambigua;
- **documentazione coerente con implementazione reale**: il documento descrive solo ciò che il repository espone oggi, senza introdurre claim su stack observability non presenti.
