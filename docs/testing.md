# Testing canonico

Questo documento è il riferimento canonico per la distinzione tra test interni in-memory, test di integrazione/end-to-end M09, test canonico M10 per crash/restart e relativi comandi operativi di validazione del repository.

## Ambito

La strategia di test corrente è organizzata su tre livelli:

- **suite repository-wide** per verificare regressioni generali su package interni;
- **test interni di convergenza in-memory** nel package `tests/gossip`, utili per verifiche rapide della logica gossip e degli scenari crash/rejoin;
- **suite di integrazione end-to-end M09** in `tests/integration`, usata come entrypoint canonico per la convergenza del cluster;
- **test canonico M10** in `tests/integration`, dedicato a crash, funzionamento del cluster residuo e rejoin del nodo riavviato.

## Test interni di convergenza in-memory (`tests/gossip`)

Le suite storiche nel package `tests/gossip` restano supportate e vanno considerate **test interni**: usano una rete in-memory, esercitano direttamente l'engine gossip e sono pensate per controlli rapidi della logica interna, non come scenario canonico di milestone.

Entry point principali:

- `TestIntegrationGossipConvergence`
- `TestCrashNodeDownClusterResidualConverges`
- `TestCrashRestartRejoinOptional`

Comandi utili:

```bash
go test ./tests/gossip -run TestIntegrationGossipConvergence -count=1
go test ./tests/gossip -run TestCrash -count=1
make test-integration-internal
make test-crash
```

Questo target resta volutamente un **entry point interno/debug**: punta ai test del package `tests/gossip` e non coincide con il comando canonico di milestone M10.

Questi test **non** vanno descritti come test end-to-end del cluster locale multi-nodo: non usano Docker Compose, non aprono porte UDP reali e non rappresentano un ambiente di deployment.

## Test di integrazione end-to-end M09

Il test canonico della milestone M09 è:

- file: `tests/integration/cluster_convergence_test.go`;
- entrypoint: `TestClusterConvergence`.

### Scenario M09

`TestClusterConvergence` avvia automaticamente **il cluster locale reale** tramite il deployment Compose canonico di root, usando `scripts/cluster_up.sh`, `scripts/cluster_wait_ready.sh` e `scripts/cluster_down.sh`. La suite quindi non usa più `newIntegrationNetwork()` né `bootstrapCluster(...)` dell'harness in-memory: parte davvero con i tre servizi `node1`, `node2`, `node3` definiti nel `docker-compose.yml` di root, attende readiness osservabile e poi raccoglie i valori finali dai log strutturati di shutdown.

La suite veloce/deterministica resta disponibile nello stesso package come `TestClusterConvergenceInMemory`, utile per debugging locale rapido senza Docker. La distinzione canonica è quindi esplicita: **M09 = cluster Compose reale**, **variante veloce = harness in-memory**.

Parametri di scenario congelati:

- **numero di nodi**: `3` (`node-1`, `node-2`, `node-3`);
- **servizi Compose reali**: `node1`, `node2`, `node3`;
- **aggregazione attiva**: `average` su tutti e tre i nodi Compose;
- **valori iniziali**: `10`, `30`, `50`, configurati nei file `configs/node1.yaml`, `configs/node2.yaml`, `configs/node3.yaml` tramite `initial_value`;
- **valore atteso informativo comune**: `30.0`, cioè `average(10, 30, 50)`;
- **criterio di successo**: la banda `max(values) - min(values)` deve risultare `<= 0.05` entro il timeout M09 Compose.



## Test canonico M10 — crash, cluster residuo e rejoin

Il test canonico della milestone M10 è:

- **nome canonico**: `TestNodeCrashAndRestart`;
- **file**: `tests/integration/node_crash_restart_test.go`;
- **package**: `tests/integration`;
- **strategia di bootstrap**: stesso **harness in-memory promosso** già usato da M09, con trasporto deterministico e membership full-mesh iniziale.

### Scenario del cluster M10

`TestNodeCrashAndRestart` usa un cluster a tre nodi con parametri congelati nel test:

- **nodi**: `node-1`, `node-2`, `node-3`;
- **aggregazione attiva**: `average`;
- **valori iniziali**: `10`, `30`, `90`;
- **nodo crashato**: `node-1`;
- **momento del crash in termini osservabili**: il crash avviene solo dopo che il test ha osservato attività gossip reale pre-crash, cioè dopo che almeno uno snapshot del cluster differisce dallo snapshot iniziale entro la finestra di bootstrap;
- **valore di restart del nodo rientrato**: `initialValues[0] + 17.0`, quindi `27.0`, scelto apposta per verificare che il nodo non resti bloccato sul valore locale di riavvio.

### Sequenza osservabile verificata da M10

Il test verifica in ordine esplicito e osservabile:

1. bootstrap del cluster multi-nodo coerente con l'architettura corrente;
2. attività gossip prima del crash tramite snapshot che cambiano realmente;
3. crash del nodo `node-1` durante i round gossip;
4. deregistrazione effettiva del nodo crashato dal transport di test;
5. convergenza del cluster residuo senza coordinatore centrale;
6. almeno tre snapshot consecutivi del cluster residuo che mostrano progresso monotono oppure stabilizzazione coerente entro banda;
7. restart del nodo crashato e sua nuova registrazione sulla rete di test;
8. rejoin reale del nodo, verificato osservando che il valore non resta bloccato sul valore di restart preimpostato;
9. convergenza stabile finale del nodo rientrato su più poll consecutivi, non su un singolo snapshot vincente;
10. confronto finale del nodo rientrato sia con la banda del cluster sia con un valore atteso informativo derivato dal cluster residuo stabile.

### Criterio per dimostrare che il cluster residuo continua a funzionare

Il cluster residuo (`node-2`, `node-3`) viene considerato ancora funzionante solo se entrambe le condizioni risultano vere:

- la convergenza del sotto-cluster viene osservata entro `crashRestartCrashTimeout`;
- tre snapshot consecutivi soddisfano una delle due proprietà esplicite: **progresso monotono** della banda (`maxDelta` non crescente) oppure **stabilizzazione coerente** entro la soglia residua.

In pratica M10 non accetta come prova un singolo snapshot favorevole: richiede polling ripetuto e una breve finestra di stabilizzazione del cluster residuo.

### Criterio per dimostrare il rejoin del nodo riavviato

Il rejoin non è dimostrato soltanto dal riavvio del processo di test. M10 richiede due evidenze osservabili:

- il nodo riavviato deve risultare nuovamente registrato nella rete di test in-memory;
- il suo valore osservato deve allontanarsi dal valore di restart di almeno `crashRestartMinimumRejoinDelta = 0.50`.

Questo evita falsi positivi in cui il nodo viene riacceso ma non riceve davvero aggiornamenti gossip dal cluster residuo.

### Criterio finale di convergenza del nodo rientrato

La convergenza finale del nodo rientrato è dimostrata solo se, entro la finestra di rejoin, il test osserva tutti i seguenti criteri:

- almeno `crashRestartStabilityPolls = 3` snapshot consecutivi stabili sull'intero cluster;
- banda finale del cluster `<= crashRestartConvergenceBand`;
- distanza del nodo rientrato dalla banda del cluster `<= crashRestartConvergenceBand`;
- distanza del nodo rientrato dal valore informativo derivato dal cluster residuo **migliore** rispetto alla distanza iniziale del valore di restart da quello stesso riferimento;
- il nodo rientrato non rimane troppo vicino al valore di restart artificiale.

Il valore informativo finale non è un numero magico hard-coded: viene derivato dal valore medio osservato nell'ultimo snapshot stabile del cluster residuo prima del restart.

### Timeout M10 motivati e non magici

I timeout M10 sono centralizzati in `tests/integration/node_crash_restart_test.go` e sono motivati dal flusso osservabile del test, non da sleep arbitrari:

- `crashRestartBootstrapTimeout = 120ms`: finestra per osservare attività gossip pre-crash reale prima di dichiarare il test non significativo;
- `crashRestartCrashTimeout = 220ms`: finestra per lasciare convergere il cluster residuo dopo il crash e raccogliere la stabilizzazione su più poll;
- `crashRestartRejoinTimeout = 320ms`: finestra più ampia per consentire restart, nuova registrazione, assorbimento degli update gossip e stabilizzazione finale del nodo rientrato;
- `crashRestartGossipInterval = 10ms` e `crashRestartPollInterval = 20ms`: granularità esplicita di propagazione e osservazione, che spiegano la scala temporale dei timeout superiori.

In sintesi, i timeout sono costruiti per coprire tre fasi diverse — bootstrap osservabile, resilienza del cluster residuo, rejoin/stabilizzazione — invece di condensare tutto in un singolo numero opaco.

### Polling e stabilizzazione espliciti

M10 usa meccanismi espliciti di polling/stabilizzazione, tutti visibili nel codice del test:

- `waitForClusterActivity(...)` per verificare che il crash avvenga dopo attività gossip osservabile;
- `waitForClusterConvergence(...)` per la convergenza del cluster residuo;
- `collectStableConvergenceSnapshots(...)` per richiedere più snapshot consecutivi stabili sia sul cluster residuo sia sul cluster completo dopo il rejoin;
- `waitForCondition(...)` per verificare il rejoin reale del nodo riavviato;
- `residualSnapshotsShowCoherentProgress(...)` per accettare solo progressi/stabilizzazioni coerenti e leggibili.

Questo rende esplicito che il test evita sleep “alla cieca” e basa il verdetto su condizioni osservabili e ripetute.

### Parametri centralizzati nel test M10

I parametri principali sono centralizzati come costanti all'inizio di `tests/integration/node_crash_restart_test.go`:

- `crashRestartNodeCount = 3`;
- `crashRestartAggregation = "average"`;
- `crashRestartGossipInterval = 10ms`;
- `crashRestartPollInterval = 20ms`;
- `crashRestartBootstrapTimeout = 120ms`;
- `crashRestartCrashTimeout = 220ms`;
- `crashRestartRejoinTimeout = 320ms`;
- `crashRestartConvergenceBand = 0.08`;
- `crashRestartResidualExpectedBand = 0.05`;
- `crashRestartStabilityPolls = 3`;
- `crashRestartResidualSnapshotCount = 3`;
- `crashRestartRestartValueOffset = 17.0`;
- `crashRestartMinimumRejoinDelta = 0.50`.

### Limiti noti del test M10

I limiti noti sono intenzionalmente esplicitati:

- la rete è **in-memory** e non esercita una rete reale;
- il trasporto non usa **UDP reale** né socket di sistema;
- il test resta soggetto a una **sensibilità residua al timing locale** della macchina/CI, pur mitigata da polling e soglie esplicite;
- la full-mesh iniziale e il delivery sincrono dell'harness privilegiano riproducibilità e diagnosi, ma non simulano tutte le varianti di un deployment reale.

### Rapporto tra test interno, test canonico M10 e script manuali

Il rapporto tra gli strumenti di verifica crash/rejoin del repository è il seguente:

- **`tests/gossip`**: i test crash/rejoin interni, inclusi entry point come `TestCrashRestartRejoinOptional`, restano verifiche rapide della logica gossip e della resilienza in-memory del package. Sono utili per sviluppo locale e debugging del merge/engine, ma non costituiscono il riferimento canonico di milestone. Il target `make test-crash` resta associato a questo livello.
- **`tests/integration`**: `TestNodeCrashAndRestart` è il **test canonico M10**. Usa ancora un harness in-memory, ma sposta il focus sul comportamento osservabile del cluster a tre nodi, con criteri espliciti di cluster residuo, rejoin e convergenza finale. I target `make test-crash-restart` e `make test-m10` puntano a questo livello canonico.
- **`scripts/fault_injection/`**: gli script manuali (`node_stop_start.sh`, `collect_debug_snapshot.sh`) non sono test canonici automatici. Servono per fault injection operativa sul cluster Docker Compose locale, raccolta artefatti e diagnosi umana di scenari crash/restart reali lato deployment.

La relazione corretta è quindi: **test interno** per la logica del package, **test canonico M10** per la milestone automatica di repository, **script manuali** per osservabilità e validazione operativa su Compose.

### Comando operativo canonico M10

```bash
go test ./tests/integration -run TestNodeCrashAndRestart -count=1
make test-crash-restart
# alias equivalente: make test-m10
```

Qui la distinzione è intenzionale:
- `make test-crash` = **target interno/debug** per gli scenari crash/rejoin storici in `tests/gossip`;
- `make test-crash-restart` = **target canonico milestone M10** per `tests/integration/TestNodeCrashAndRestart`;
- `make test-m10` = alias leggibile del target canonico M10.

### Timeout operativo

I timeout ufficiali di M09 sono ora separati in modo esplicito tra suite reale e suite veloce:

- **suite canonica Compose**: `composeReadyTimeout = 90s`, `composeConvergenceTimeout = 18s`, `composeShutdownTimeout = 40s`;
- **suite veloce in-memory**: `m09InMemoryTimeout = 350ms`.

Motivazione operativa della suite Compose:

- `90s` coprono build immagine, bootstrap container e marker osservabili nei log (`gossip bootstrap completato`, `transport gossip avviato`) senza dipendere da sleep ciechi;
- `18s` lasciano al cluster reale con `gossip_interval_ms = 1000` una finestra di convergenza di più round completi prima del teardown controllato;
- `40s` riservano margine per stop pulito dei container e raccolta degli artefatti finali di shutdown.

La suite in-memory mantiene invece il timeout corto storico da `350ms` per debugging rapido e riproducibile.

### Parametri centralizzati nel test

I parametri M09 sono facilmente rintracciabili perché centralizzati come costanti all’inizio di `tests/integration/cluster_convergence_test.go` e `tests/integration/compose_harness_test.go`:

- `m09NodeCount = 3`;
- `m09Aggregation = "average"`;
- `m09ComposeTimeout = 18s`;
- `m09ComposePollInterval = 1s`;
- `m09InMemoryGossipInterval = 10ms`;
- `m09InMemoryPollInterval = 20ms`;
- `m09InMemoryTimeout = 350ms`;
- `m09ConvergenceBand = 0.05`;
- `composeReadyTimeout = 90s`;
- `composeReadyPollInterval = 2s`;
- `composeShutdownTimeout = 40s`.

### Formato del report finale

Il report finale emesso via `t.Logf` mantiene, per ogni nodo, il formato M09 già usato da `formatNodeObservation(...)`. Nella suite Compose i valori vengono prima estratti dai log strutturati `shutdown nodo completato` raccolti in `artifacts/cluster/latest-final-values.txt`, poi ricostruiti nello stesso `clusterObservation` usato dal report.

```text
node_id=<id> observed_value=<valore> expected_delta=<differenza_dal_valore_atteso> common_band=<banda_cluster>
```

Dove:

- `node_id` identifica il nodo osservato;
- `observed_value` è il valore finale letto nello snapshot;
- `expected_delta` è la differenza assoluta dal valore atteso comune (`30.0` nello scenario corrente);
- `common_band` è la banda comune del cluster al momento del report.

## Verifica canonica dei casi rischiosi M02

Per il consolidamento M02 i casi rischiosi dichiarati nel task vengono verificati principalmente nel package `tests/gossip`, con un supporto mirato da `tests/integration` per la failure detection runtime, così da coprire sia le regole di merge membership sia il comportamento osservabile dei timeout membership.

### Scenari coperti

- **Partizione temporanea tra sottoinsiemi di nodi con riconvergenza membership**: `tests/gossip/TestMergeMembershipReconvergesAfterTemporaryPartition` modella due sottoinsiemi che sviluppano viste membership divergenti durante la partizione e verifica che, alla chiusura della partizione, update `alive` con `incarnation` maggiore riallineino entrambi i lati sulla stessa membership finale.
- **Recupero da `suspect` tramite update gossip `alive` con `incarnation` maggiore**: `tests/gossip/TestMergeMembershipRecoversSuspectWithHigherAliveIncarnation` congela il caso di falso positivo di failure detection e verifica che l’update più fresco riattivi correttamente il peer.
- **Rejoin con stato obsoleto ignorato**: `tests/gossip/TestMergeMembershipIgnoresRejoinWithLowerIncarnation` verifica che, dopo prune di un tombstone più nuovo, un update `alive` con `incarnation` minore non possa reintrodurre il nodo.
- **Distinzione tra placeholder seed `host:port` e vero `node_id`**: `tests/gossip/TestMergeMembershipRealignsPlaceholderSeedWithCanonicalNodeID` verifica il riallineamento dal placeholder seed-only al `node_id` canonico propagato via gossip, senza mantenere duplicati logici nella membership.

### Comandi canonici M02

Separiamo esplicitamente gli entrypoint per evitare riferimenti fuorvianti a package interni senza file `*_test.go` e per rendere chiaro il livello di garanzia ottenuto.

```bash
# verifica unitaria membership
go test ./tests/membership -run 'TestJoinLeave|TestTimeoutTransitions|TestPruneRemovesExpiredDeadPeerAndBlocksObsoleteReintroduction' -count=1

# verifica gossip membership
go test ./tests/gossip -run 'TestMergeMembership|TestRoundSerializzaMembershipConIncarnation' -count=1

# eventuale verifica integrazione runtime
go test ./tests/integration -run TestRuntimeMembershipFailureDetection -count=1
```

Se serve analizzare un singolo rischio M02, restano disponibili anche i comandi puntuali sui test gossip dedicati:

```bash
go test ./tests/gossip -run TestMergeMembershipReconvergesAfterTemporaryPartition -count=1
go test ./tests/gossip -run TestMergeMembershipRecoversSuspectWithHigherAliveIncarnation -count=1
go test ./tests/gossip -run TestMergeMembershipIgnoresRejoinWithLowerIncarnation -count=1
go test ./tests/gossip -run TestMergeMembershipRealignsPlaceholderSeedWithCanonicalNodeID -count=1
```

## Test canonico observability

La suite esterna `tests/observability` include ora il test canonico:

- **nome canonico**: `TestMetricsExposure`;
- **file**: `tests/observability/metrics_test.go`.

La suite verifica in modo deterministico che:

- l'endpoint `/metrics` esponga almeno le metriche minime del nodo (`rounds`, merge remoti per esito, peer noti, stima corrente, uptime, readiness);
- l'endpoint `/health` risponda positivamente con HTTP `200 OK`;
- l'endpoint `/ready` rifletta coerentemente lo stato del collector restituendo `503` quando il nodo non è pronto e `200` quando viene marcato ready;
- gli esiti di merge non riconosciuti vengano collassati nel bucket stabile `unknown`, evitando label ad alta cardinalità.

Comando operativo mirato:

```bash
go test ./tests/observability -run TestMetricsExposure -count=1
```

## Test di integrazione bootstrap via join endpoint reale

La suite di integrazione include anche il test mirato:

- **nome canonico**: `TestNodeBootstrapViaJoinEndpointPopulatesInitialMembership`;
- **file**: `tests/integration/join_endpoint_bootstrap_test.go`.

Scenario verificato:

- il test avvia un endpoint HTTP di join reale con `httptest`;
- il processo `go run ./cmd/node` viene eseguito con `join_endpoint` valorizzato e senza peer statici di bootstrap;
- il server di join restituisce una `JoinResponse` con uno snapshot membership iniziale contenente un peer UDP reale;
- il test considera il bootstrap corretto solo se osserva sia la `JoinRequest` HTTP inviata dal nodo sia almeno un payload gossip UDP verso il peer restituito dal join endpoint.

Comando operativo mirato:

```bash
go test ./tests/integration -run TestNodeBootstrapViaJoinEndpointPopulatesInitialMembership -count=1
```

## Helper script per cluster locale Docker Compose

Per la validazione operativa/manuale del cluster locale multi-nodo con Docker Compose, il repository ora include helper minimi in `scripts/` progettati per essere **idempotenti**, robusti rispetto a container residui e leggibili in caso di errore:

- `scripts/cluster_up.sh`: cleanup preventivo del progetto Compose canonico e avvio del cluster con build locale;
- `scripts/cluster_wait_ready.sh`: attesa dello stato operativo verificando sia `running` dei container sia la presenza nei log di `gossip bootstrap completato` e `transport gossip avviato`;
- `scripts/cluster_collect_results.sh`: raccolta di `docker compose ps`, log aggregati e ultimo report di valori finali disponibile nei log;
- `scripts/cluster_down.sh`: stop pulito del cluster, raccolta degli artefatti finali e `docker compose down --remove-orphans`.
- `scripts/fault_injection/node_stop_start.sh`: stop/start/bounce di un singolo nodo Compose per prove manuali di crash/restart;
- `scripts/fault_injection/collect_debug_snapshot.sh`: snapshot diagnostici minimi per un nodo target in `artifacts/fault_injection/`.

Gli script usano naming prevedibile e stabile:

- file Compose canonico: `docker-compose.yml` alla root;
- project name Compose: `sdcc-bootstrap`;
- directory artefatti: `artifacts/cluster/`;
- symlink aggiornati automaticamente: `latest-compose-ps.txt`, `latest-cluster-logs.log`, `latest-final-values.txt`.

Flusso operativo consigliato:

```bash
scripts/cluster_up.sh
scripts/cluster_wait_ready.sh
scripts/cluster_collect_results.sh
scripts/cluster_down.sh
```

### Fault injection operativo/manuale sul cluster Compose

Per scenari manuali di crash/restart il repository include la directory `scripts/fault_injection/`, volutamente minimale e senza coordinatore centrale. Gli script riusano `scripts/cluster_common.sh`, quindi restano allineati al project name Compose canonico, al file `docker-compose.yml` di root e ai servizi `node1`, `node2`, `node3`.

Esempio operativo suggerito:

```bash
scripts/cluster_up.sh
scripts/cluster_wait_ready.sh
AFTER_STOP_SLEEP_SECONDS=5 scripts/fault_injection/node_stop_start.sh bounce node2
SNAPSHOT_LABEL=post-bounce scripts/fault_injection/collect_debug_snapshot.sh node2
scripts/cluster_down.sh
```

Parametri principali supportati:

- `ACTION`, `SERVICE`, `STOP_TIMEOUT_SECONDS`, `START_TIMEOUT_SECONDS`, `AFTER_STOP_SLEEP_SECONDS`, `WAIT_FOR_RUNNING` per `node_stop_start.sh`;
- `SERVICE`, `LOG_TAIL_LINES`, `SNAPSHOT_LABEL` per `collect_debug_snapshot.sh`.

Gli artefatti vengono salvati in `artifacts/fault_injection/` con un symlink `latest-<service>` verso l'ultimo snapshot raccolto per il nodo target.

Nota esplicita di scope: il test automatico canonico crash/restart resta `TestNodeCrashAndRestart` dentro `tests/integration` e continua a usare un harness in-memory; gli script `scripts/fault_injection/` sono solo supporti operativi/manuali per osservare o diagnosticare il cluster Docker Compose locale.

Note operative importanti:

- `cluster_up.sh` esegue sempre un cleanup preventivo, quindi può essere rilanciato in ambiente sporco senza richiedere interventi manuali;
- i **valori finali** per nodo vengono estratti dai log applicativi prodotti in shutdown con il messaggio strutturato `shutdown nodo completato`;
- per questo motivo il file `artifacts/cluster/latest-final-values.txt` contiene il riepilogo finale completo soprattutto dopo `scripts/cluster_down.sh`;
- se il cluster non è ancora stato fermato, `cluster_collect_results.sh` salva comunque i log correnti e segnala esplicitamente l'assenza del riepilogo finale.

## Comandi operativi canonici

### Verifica mirata M09

```bash
go test ./tests/integration -run TestClusterConvergence -count=1
```

Questo è il comando ufficiale da usare per validare la convergenza del cluster introdotta dalla milestone M09. Il target equivalente del `Makefile` è `make test-integration`.

### Verifica mirata M10

```bash
go test ./tests/integration -run TestNodeCrashAndRestart -count=1
```

Questo è il comando operativo canonico da usare per validare lo scenario M10 di crash, continuità del cluster residuo e rejoin del nodo riavviato.

### Verifica repository-wide

```bash
go test ./... -run Test -count=1
```

Questo comando resta utile per confermare che il test M09 non introduca regressioni sulle suite esistenti.

## Note operative

- La suite `tests/integration` usa una rete in-memory e non richiede Docker, porte UDP reali o servizi esterni.
- Per evitare ambiguità terminologiche: **test interni di convergenza in-memory** = suite in `tests/gossip` e target `make test-crash`; **test di integrazione/end-to-end M09** = `TestClusterConvergence` in `tests/integration`; **test canonico M10** = `TestNodeCrashAndRestart` in `tests/integration` e target `make test-crash-restart` / `make test-m10`; **cluster locale multi-nodo con Docker Compose** = scenario operativo/manuale distinto, utile per validazione di deployment ma non eseguito da questa suite automatica.
- Il bootstrap del cluster è automatico nel test e costruisce i tre nodi `node-1`, `node-2`, `node-3` con membership full-mesh iniziale.
- Il polling usa `time.NewTicker` e un timeout esplicito, evitando sleep arbitrari.
- In caso di success o failure, il test emette un report leggibile tramite `t.Logf` con valori finali per nodo e metriche di convergenza.


Nota runtime: `cmd/node/main.go` avvia anche il piccolo server HTTP di observability sul binding `OBSERVABILITY_ADDR` se presente, altrimenti `:8080`.

## Failure detection runtime nel cluster di test
Lo scenario `TestRuntimeMembershipFailureDetection` verifica che il loop gossip degradi automaticamente un peer fermato dal runtime del cluster di test senza invocare manualmente `ApplyTimeoutTransitions` dal test stesso. Il test riduce i timeout membership per mantenere la suite rapida e osserva, su un nodo superstite, la sequenza `alive -> suspect -> dead` del peer inattivo dopo lo stop del relativo engine.

Comando operativo dedicato:
```bash
go test ./tests/integration -run TestRuntimeMembershipFailureDetection -count=1
```
