# Testing canonico

Questo documento è il riferimento canonico per la distinzione tra test interni in-memory, test di integrazione/end-to-end M09, test canonico M10 per crash/restart e relativi comandi operativi di validazione del repository.

Per la guida demo operativa del cluster (setup, osservazioni, criteri di successo e troubleshooting), vedere anche `docs/demo.md`.

## Verifiche operative per demo/documentazione M12

Per lo scenario dimostrativo end-to-end fare riferimento a `docs/demo.md`, mentre questa pagina resta il riferimento canonico per il perimetro e i livelli di test.

- **Verifiche locali (Docker Compose)**: usare i comandi canonici già documentati in README/testing per avvio cluster e verifiche M09/M10 (`docker compose up -d --build`, `docker compose ps`, `go test ./tests/integration -run TestClusterConvergence -count=1`, `go test ./tests/integration -run TestNodeCrashAndRestart -count=1`, `docker compose down`).
- **Note deploy EC2 (stesso stack, ambiente diverso)**: usare lo stesso stack Compose descritto in `docs/deployment_ec2.md`, considerando differenze operative di rete/host (latenza, porte, endpoint locali alla VM) e i limiti pratici di tempo/costi Learner Lab già riportati in quel documento.
- **Limiti pratici da richiamare in demo/documentazione**: lo scenario crash/restart va dichiarato solo dove supportato dai test reali (`TestNodeCrashAndRestart` su Compose e variante `TestNodeCrashAndRestartInMemory`); evitare di estendere il claim a percorsi non coperti da suite esistenti.

Riferimenti incrociati per evitare ridondanza: `README.md` (quickstart/comandi), `docs/demo.md` (scenari e osservazioni), `docs/deployment_ec2.md` (vincoli EC2).

## Ambito

La strategia di test corrente è organizzata su tre livelli:

- **suite repository-wide** per verificare regressioni generali su package interni;
- **test interni di convergenza in-memory** nel package `tests/gossip`, utili per verifiche rapide della logica gossip e degli scenari crash/rejoin;
- **suite di integrazione end-to-end M09** in `tests/integration`, usata come entrypoint canonico per la convergenza del cluster;
- **test rapido in-memory M10** in `tests/integration`, utile per debugging locale rapido del flusso crash/restart;
- **test lento/reale M10 Compose** in `tests/integration`, dedicato a crash, funzionamento del cluster residuo e rejoin del nodo riavviato su cluster locale reale.

## Concurrency checks

Per consolidare il comportamento concorrente del runtime gossip/membership sono disponibili test dedicati che stressano accessi simultanei su strutture condivise.

Copertura introdotta:

- `tests/membership/TestConcurrentSetOperations`: goroutine concorrenti che invocano `Upsert`, `Touch`, `LeaveAt` e `Snapshot` sullo stesso `membership.Set`.
- `tests/gossip/TestRoundOnceConcurrentWithRemoteDelivery`: esecuzione concorrente di `RoundOnce` con delivery di messaggi remoti simulati via transport spy.
- `tests/gossip/TestConcurrentRoundOnceAndRemoteDeliveryInvariants`: verifica invarianti concorrenti su deduplica canonico/alias e monotonicità `incarnation`.

Comandi consigliati:

```bash
# verifica base concorrente senza race detector
go test ./tests/membership ./tests/gossip -run 'TestConcurrentSetOperations|TestRoundOnceConcurrentWithRemoteDelivery|TestConcurrentRoundOnceAndRemoteDeliveryInvariants' -count=1

# verifica opzionale con race detector (dove supportato dalla toolchain/piattaforma)
go test -race ./tests/membership ./tests/gossip -run 'TestConcurrentSetOperations|TestRoundOnceConcurrentWithRemoteDelivery|TestConcurrentRoundOnceAndRemoteDeliveryInvariants' -count=1
```

Nota operativa:
- `-race` è raccomandato per ambienti locali/CI che supportano il race detector Go; in ambienti limitati può essere omesso mantenendo comunque il comando base.

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



## Test M10 — variante rapida in-memory e suite reale Compose

Per M10 il repository distingue ora in modo esplicito due livelli complementari:

- **variante rapida/deterministica in-memory**: `tests/integration/TestNodeCrashAndRestartInMemory`;
- **suite lenta/reale Compose**: `tests/integration/TestNodeCrashAndRestart`.

La variante in-memory resta utile come controllo rapido di debug locale. La suite `TestNodeCrashAndRestart` è invece la verifica automatica richiesta per il crash/restart su **cluster locale reale** orchestrato via Docker Compose e pilotato dagli script canonici della repository.

### Variante rapida in-memory (`TestNodeCrashAndRestartInMemory`)

Questa suite conserva il vecchio harness `tests/integration/harness_test.go` e continua a usare rete/transport in-memory per esercitare in modo riproducibile:

- crash del nodo `node-1`;
- convergenza del cluster residuo;
- restart del nodo fermato;
- verifica del rejoin e della stabilizzazione finale.

È il test più veloce da usare durante lo sviluppo, ma non costituisce più da solo la prova completa del requisito M10 su deployment reale.

Comando rapido:

```bash
go test ./tests/integration -run TestNodeCrashAndRestartInMemory -count=1
make test-crash-restart-internal
```

### Suite lenta/reale Compose (`TestNodeCrashAndRestart`)

Il test automatico canonico M10 è ora:

- **nome canonico**: `TestNodeCrashAndRestart`;
- **file**: `tests/integration/node_crash_restart_compose_test.go`;
- **harness reale**: `tests/integration/compose_harness_test.go`;
- **bootstrap cluster**: `scripts/cluster_up.sh` + `scripts/cluster_wait_ready.sh`;
- **fault injection reale**: `scripts/fault_injection/node_stop_start.sh`;
- **raccolta artefatti**: `scripts/fault_injection/collect_debug_snapshot.sh` + `scripts/cluster_down.sh`.

#### Scenario Compose verificato

`TestNodeCrashAndRestart` esegue in ordine osservabile:

1. avvio del cluster locale reale `node1` / `node2` / `node3` dal `docker-compose.yml` canonico;
2. attesa della readiness tramite marker di bootstrap/log e endpoint HTTP;
3. acquisizione di uno snapshot live pre-crash via endpoint `/metrics`;
4. stop reale di `node1` tramite `scripts/fault_injection/node_stop_start.sh stop node1`;
5. raccolta di uno snapshot diagnostico `after-stop` con `scripts/fault_injection/collect_debug_snapshot.sh`;
6. verifica che il cluster residuo (`node2`, `node3`) resti osservabile e continui a completare round gossip reali;
7. restart reale di `node1` tramite `scripts/fault_injection/node_stop_start.sh start node1`;
8. raccolta di uno snapshot diagnostico `after-restart`;
9. verifica del rejoin tramite endpoint `/ready` e `/metrics`, richiedendo che il nodo riavviato completi round gossip e si allontani in modo osservabile dal proprio `initial_value`;
10. verifica della riconvergenza finale dell’intero cluster sia via snapshot live sia via valori finali di shutdown raccolti nel teardown controllato.

#### Evidenze osservabili richieste

La suite reale non si limita a invocare gli script: richiede evidenze esplicite e leggibili.

- **cluster residuo**: `node2` e `node3` devono restare `ready` e mostrare `sdcc_node_rounds_total` crescente rispetto al baseline pre-crash;
- **rejoin del nodo fermato**: `node1` deve tornare `ready`, esporre `sdcc_node_rounds_total > 0` dopo il restart e mostrare una `sdcc_node_estimate` che non resti bloccata sul valore iniziale `10.0`;
- **artefatti**: il test salva snapshot diagnostici `after-stop` e `after-restart` in `artifacts/fault_injection/`;
- **stato finale**: il teardown `cluster_down.sh` deve produrre uno snapshot finale coerente e convergente in `artifacts/cluster/latest-final-values.txt`.

#### Comandi canonici M10

```bash
go test ./tests/integration -run TestNodeCrashAndRestart -count=1
go test ./tests/integration -run TestNodeCrashAndRestartInMemory -count=1
make test-crash-restart
make test-crash-restart-internal
# alias equivalente del test reale: make test-m10
```

#### Rapporto tra i livelli di verifica M10

- **`tests/gossip`**: test interni storici di logica/package;
- **`tests/integration/TestNodeCrashAndRestartInMemory`**: variante rapida per debugging locale del flusso M10;
- **`tests/integration/TestNodeCrashAndRestart`**: test automatico canonico M10 su cluster Compose reale;
- **script `scripts/fault_injection/`**: supporto operativo riusato dal test reale e utile anche per diagnosi manuale.

Questa distinzione evita ambiguità: il controllo veloce resta disponibile, ma il requisito di crash/restart reale è coperto da una suite automatica separata e più lenta.

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
- **Filtro esplicito del self node**: `tests/membership/TestApplyTimeoutTransitionsSaltaSempreSelfNode`, `tests/gossip/TestRoundSerializzaMembershipEscludendoSelfNode`, `tests/gossip/TestMergeMembershipIgnoraEntryDelNodoLocale` e `tests/gossip/TestRoundNonLoggaTimeoutPerSelfNode` verificano che il nodo locale non transizioni via timeout, non venga serializzato nel digest e non produca eventi/log di timeout auto-riferiti.

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

## Regressioni canoniche average

Per l'aggregazione `average` il repository congela esplicitamente due rischi:

- il contributo locale di un nodo non deve driftare verso la media corrente del cluster dopo round multipli;
- un payload remoto che include gia' metadata `average` completi non deve re-inferire il contributo del mittente a partire da `state.value`.

Comandi mirati:

```bash
go test ./tests/aggregation/average -run 'TestAverageConvergence|TestAverageRoundDoesNotDriftLocalContribution' -count=1
go test ./tests/gossip -run 'TestAverageRoundPreservaContributoLocaleOriginario|TestMergeAverageNonReinferisceContributoDaRemoteValueQuandoMetadataCompleti' -count=1
```

## Test canonico observability

La suite esterna `tests/observability` include ora il test canonico:

- **nome canonico**: `TestMetricsExposure`;
- **file**: `tests/observability/metrics_test.go`.

La suite verifica in modo deterministico che:

- l'endpoint `/metrics` esponga almeno le metriche minime del nodo (`rounds`, merge remoti per esito, peer noti, stima corrente, uptime, readiness);
- il collector condiviso con l'engine aggiorni davvero le metriche durante l'esecuzione del nodo: i round devono far crescere `sdcc_node_rounds_total`, i merge remoti devono incrementare `sdcc_node_remote_merges_total{result=...}` e le gauge `known_peers`/`estimate` devono riflettere lo stato locale post-round/post-merge, non solo bootstrap o shutdown;
- l'endpoint `/health` risponda positivamente con HTTP `200 OK`;
- l'endpoint `/ready` rifletta coerentemente lo stato del collector restituendo `503` quando il nodo non è pronto e `200` quando viene marcato ready;
- gli esiti di merge non riconosciuti vengano collassati nel bucket stabile `unknown`, evitando label ad alta cardinalità.

Comando operativo mirato:

```bash
go test ./tests/observability -run TestMetricsExposure -count=1
```

## Regressioni canonicalizzazione endpoint origine gossip

Per evitare dipendenze da `remoteAddr` UDP non canonico, il repository include casi mirati su:

- fallback non canonico da transport (`remoteAddr`) quando il digest non contiene l'origin;
- presenza di `origin_addr` nel metadata gossip;
- stabilità del numero di entry membership in cluster 3 nodi (tipicamente 2 remote per digest locale, escluso self).

Comandi mirati:

```bash
go test ./tests/gossip -run 'TestEngineIgnoraFallbackRemoteAddrNonCanonicoQuandoDigestNonHaOrigin|TestRoundIncludeOriginAddrInMetadataPerRendereAffidabileEndpointOrigine' -count=1
go test ./tests/integration -run TestMembershipEntriesRestanoStabiliNelCluster3Nodi -count=1
go test ./tests/transport -run TestUDPTransportSendUsaSocketPersistenteConRemoteAddrStabile -count=1
```

## Test di integrazione bootstrap via join endpoint reale

La suite di integrazione include anche il test mirato:

- **nome canonico**: `TestNodeBootstrapViaJoinEndpointPopulatesInitialMembership`;
- **file**: `tests/integration/join_endpoint_bootstrap_test.go`.

Scenario verificato:

- il test avvia un endpoint HTTP di join reale con `httptest`;
- il test costruisce prima un binario temporaneo del nodo da `./cmd/node` tramite l'helper `buildNodeBinary(...)`;
- il processo viene poi eseguito con `exec.CommandContext(...)` sul binario generato, passando `--config <file temporaneo>` con `join_endpoint` valorizzato e senza peer statici di bootstrap;
- l'ambiente del processo imposta esplicitamente `OBSERVABILITY_ADDR=127.0.0.1:0` per evitare collisioni sulle porte osservability durante la suite;
- il server di join restituisce una `JoinResponse` con uno snapshot membership iniziale contenente un peer UDP reale;
- il test considera il bootstrap corretto solo se osserva sia la `JoinRequest` HTTP inviata dal nodo sia almeno un payload gossip UDP verso il peer restituito dal join endpoint nella `JoinResponse`.

Comando operativo mirato:

```bash
go test ./tests/integration -run TestNodeBootstrapViaJoinEndpointPopulatesInitialMembership -count=1
```

## Helper script per cluster locale Docker Compose

Per la validazione operativa/manuale del cluster locale multi-nodo con Docker Compose, il repository ora include helper minimi in `scripts/` progettati per essere **idempotenti**, robusti rispetto a container residui e leggibili in caso di errore:

- `scripts/cluster_up.sh`: cleanup preventivo del progetto Compose canonico e avvio del cluster con build locale; se `docker compose up -d --build` fallisce, lo script stampa il comando Compose usato, l'output di `docker compose ps`, un tail dei log di `node1`/`node2`/`node3` e classifica esplicitamente i casi `plugin compose assente`, `build immagine fallita` e `container avviati ma unhealthy`;
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

Nota esplicita di scope: il test automatico canonico crash/restart è ora `TestNodeCrashAndRestart` dentro `tests/integration` e usa un harness Compose reale; la variante `TestNodeCrashAndRestartInMemory` resta disponibile per debugging rapido. Gli script `scripts/fault_injection/` sono sia supporti manuali sia dipendenze operative della suite reale M10.

Note operative importanti:

- `cluster_up.sh` esegue sempre un cleanup preventivo, quindi può essere rilanciato in ambiente sporco senza richiedere interventi manuali;
- i **valori finali** per nodo vengono estratti dai log applicativi prodotti in shutdown con il messaggio strutturato `shutdown nodo completato`;
- per questo motivo il file `artifacts/cluster/latest-final-values.txt` contiene un **record finale univoco per ogni `node_id`**, ottenuto mantenendo deterministicamente solo l'occorrenza con timestamp `time` più recente per nodo (in caso di pari timestamp viene mantenuta l'ultima riga incontrata nei log);
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

- La suite `tests/integration` contiene ora sia test in-memory sia test Compose reali: alcuni entry point non richiedono Docker, mentre `TestClusterConvergence` e `TestNodeCrashAndRestart` dipendono dal cluster locale reale.
- Per evitare ambiguità terminologiche: **test interni di convergenza in-memory** = suite in `tests/gossip` e target `make test-crash`; **test di integrazione/end-to-end M09** = `TestClusterConvergence` in `tests/integration`; **variante rapida M10** = `TestNodeCrashAndRestartInMemory`; **test canonico M10 reale** = `TestNodeCrashAndRestart` e target `make test-crash-restart` / `make test-m10`; **cluster locale multi-nodo con Docker Compose** = ambiente effettivamente usato dalla suite automatica reale M09/M10.
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
