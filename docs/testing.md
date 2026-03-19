# Testing canonico

Questo documento è il riferimento canonico per i test di integrazione e per i comandi operativi di validazione del repository.

## Ambito

La strategia di test corrente è organizzata su due livelli:

- **suite repository-wide** per verificare regressioni generali su package interni;
- **suite di integrazione dedicate** per verificare proprietà osservabili del cluster gossip, in particolare la convergenza M09.

## Test di integrazione M09

Il test di integrazione canonico della milestone M09 è:

- `tests/integration/cluster_convergence_test.go`
- test entrypoint: `TestClusterConvergence`

### Obiettivo del test

`TestClusterConvergence` avvia automaticamente un cluster a tre nodi usando la strategia scelta per M09, cioè un **harness in-memory promosso** con trasporto deterministico e membership full-mesh iniziale. Il test verifica che il cluster converga senza rete reale entro la finestra operativa dichiarata.

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

Questo è il comando ufficiale da usare per validare la convergenza del cluster introdotta dalla milestone M09.

### Verifica repository-wide

```bash
go test ./... -run Test -count=1
```

Questo comando resta utile per confermare che il test M09 non introduca regressioni sulle suite esistenti.

## Note operative

- La suite `tests/integration` usa una rete in-memory e non richiede Docker, porte UDP reali o servizi esterni.
- Il bootstrap del cluster è automatico nel test e costruisce i tre nodi `node-1`, `node-2`, `node-3` con membership full-mesh iniziale.
- Il polling usa `time.NewTicker` e un timeout esplicito, evitando sleep arbitrari.
- In caso di success o failure, il test emette un report leggibile tramite `t.Logf` con valori finali per nodo e metriche di convergenza.
