# Testing canonico

Questo documento è il riferimento canonico per la distinzione tra test interni in-memory, test di integrazione/end-to-end M09 e relativi comandi operativi di validazione del repository.

## Ambito

La strategia di test corrente è organizzata su tre livelli:

- **suite repository-wide** per verificare regressioni generali su package interni;
- **test interni di convergenza in-memory** nel package `internal/gossip`, utili per verifiche rapide della logica gossip e degli scenari crash/rejoin;
- **suite di integrazione end-to-end M09** in `tests/integration`, usata come entrypoint canonico per la convergenza del cluster.

## Test interni di convergenza in-memory (`internal/gossip`)

Le suite storiche nel package `internal/gossip` restano supportate e vanno considerate **test interni**: usano una rete in-memory, esercitano direttamente l'engine gossip e sono pensate per controlli rapidi della logica interna, non come scenario canonico di milestone.

Entry point principali:

- `TestIntegrationGossipConvergence`
- `TestCrashNodeDownClusterResidualConverges`
- `TestCrashRestartRejoinOptional`

Comandi utili:

```bash
go test ./internal/gossip -run TestIntegrationGossipConvergence -count=1
go test ./internal/gossip -run TestCrash -count=1
make test-integration-internal
make test-crash
```

Questi test **non** vanno descritti come test end-to-end del cluster locale multi-nodo: non usano Docker Compose, non aprono porte UDP reali e non rappresentano un ambiente di deployment.

## Test di integrazione end-to-end M09

Il test canonico della milestone M09 è:

- `tests/integration/cluster_convergence_test.go`
- test entrypoint: `TestClusterConvergence`

### Scenario M09

`TestClusterConvergence` avvia automaticamente lo scenario M09 usando la strategia scelta per la milestone, cioè un **harness in-memory promosso** con trasporto deterministico e membership full-mesh iniziale. Nel repository questa suite viene classificata come **integrazione/end-to-end M09** perché valida il comportamento osservabile del cluster come scenario di milestone, pur senza usare rete reale. Il test verifica quindi la convergenza end-to-end della logica di cluster, ma **non** sostituisce una prova manuale su cluster locale multi-nodo con Docker Compose.

Parametri di scenario congelati:

- **numero di nodi**: `3` (`node-1`, `node-2`, `node-3`);
- **aggregazione attiva**: `average`;
- **valori iniziali**: `10`, `30`, `50`;
- **valore atteso informativo comune**: `30.0`, cioè `average(10, 30, 50)`;
- **criterio di successo**: la banda `max(values) - min(values)` deve risultare `<= 0.05` entro il timeout M09.

### Timeout operativo

Il timeout ufficiale del test M09 è:

- `350ms`

Motivazione operativa:

- il parametro è derivato in modo esplicito da costanti centralizzate in `tests/integration/cluster_convergence_test.go`;
- con `gossip_interval_ms = 10ms`, il test riserva `50ms` di bootstrap (`5 * gossip_interval`) per dare tempo all’avvio del cluster e ai primi round utili dopo la registrazione dei transport;
- aggiunge poi un buffer di `300ms` (`15 * poll_interval`, con `poll_interval = 20ms`) per assorbire la variabilità locale e CI senza allungare inutilmente la suite;
- il totale `350ms` rimane abbastanza stretto da segnalare regressioni reali, ma più motivato e facilmente manutenibile di un valore letterale isolato.

### Parametri centralizzati nel test

I parametri M09 sono facilmente rintracciabili perché centralizzati come costanti all’inizio di `tests/integration/cluster_convergence_test.go`:

- `m09NodeCount = 3`;
- `m09Aggregation = "average"`;
- `m09GossipInterval = 10ms`;
- `m09PollInterval = 20ms`;
- `m09BootstrapAllowance = 50ms`;
- `m09LocalCIBuffer = 300ms`;
- `m09Timeout = 350ms`;
- `m09ConvergenceBand = 0.05`.

### Formato del report finale

Il report finale emesso via `t.Logf` include, per ogni nodo, il formato M09:

```text
node_id=<id> observed_value=<valore> expected_delta=<differenza_dal_valore_atteso> common_band=<banda_cluster>
```

Dove:

- `node_id` identifica il nodo osservato;
- `observed_value` è il valore finale letto nello snapshot;
- `expected_delta` è la differenza assoluta dal valore atteso comune (`30.0` nello scenario corrente);
- `common_band` è la banda comune del cluster al momento del report.

## Comandi operativi canonici

### Verifica mirata M09

```bash
go test ./tests/integration -run TestClusterConvergence -count=1
```

Questo è il comando ufficiale da usare per validare la convergenza del cluster introdotta dalla milestone M09. Il target equivalente del `Makefile` è `make test-integration`.

### Verifica repository-wide

```bash
go test ./... -run Test -count=1
```

Questo comando resta utile per confermare che il test M09 non introduca regressioni sulle suite esistenti.

## Note operative

- La suite `tests/integration` usa una rete in-memory e non richiede Docker, porte UDP reali o servizi esterni.
- Per evitare ambiguità terminologiche: **test interni di convergenza in-memory** = suite in `internal/gossip`; **test di integrazione/end-to-end M09** = suite canonica in `tests/integration`; **cluster locale multi-nodo con Docker Compose** = scenario operativo/manuale distinto, utile per validazione di deployment ma non eseguito da questa suite automatica.
- Il bootstrap del cluster è automatico nel test e costruisce i tre nodi `node-1`, `node-2`, `node-3` con membership full-mesh iniziale.
- Il polling usa `time.NewTicker` e un timeout esplicito, evitando sleep arbitrari.
- In caso di success o failure, il test emette un report leggibile tramite `t.Logf` con valori finali per nodo e metriche di convergenza.
