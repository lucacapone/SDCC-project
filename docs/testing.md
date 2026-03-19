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

### Obiettivo del test

`TestClusterConvergence` avvia automaticamente un cluster a tre nodi usando la strategia scelta per M09, cioè un **harness in-memory promosso** con trasporto deterministico e membership full-mesh iniziale. Nel repository questa suite viene classificata come **integrazione/end-to-end M09** perché valida il comportamento osservabile del cluster come scenario di milestone, pur senza usare rete reale. Il test verifica quindi la convergenza end-to-end della logica di cluster, ma **non** sostituisce una prova manuale su cluster locale multi-nodo con Docker Compose.

### Criterio di convergenza

Il cluster è considerato convergente quando la **banda massima di differenza tra i nodi** risulta:

- `<= 0.05`

La misura è calcolata come:

```text
max(values) - min(values)
```

sui valori correnti del cluster al momento del campionamento. Il test continua comunque a riportare anche il riferimento informativo `average(10, 30, 50) = 30.0`, ma senza usarlo come vincolo di pass/fail.

### Timeout operativo

Il timeout ufficiale del test M09 è:

- `2s`

Motivazione operativa:

- mantiene allineamento con i criteri quantitativi già dichiarati nel README;
- replica il riferimento logico già usato in `internal/gossip/integration_test.go`, cioè round da `10ms` e polling con ticker da `20ms` senza sleep arbitrari;
- resta abbastanza breve da segnalare regressioni reali senza rallentare inutilmente la suite.

### Parametri congelati dal test

Il test usa i seguenti parametri fissi:

- 3 nodi (`node-1`, `node-2`, `node-3`)
- valori iniziali: `10`, `30`, `50`
- aggregazione: `average`
- intervallo round gossip: `10ms`
- soglia di convergenza: `0.05`
- timeout massimo: `2s`
- polling di convergenza: `20ms`
- report finale: emissione via `t.Logf` dei valori per nodo, media iniziale di riferimento, banda cluster e offset massimo dal riferimento

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
