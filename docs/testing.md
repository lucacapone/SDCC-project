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

`TestClusterConvergence` avvia un cluster in-memory a tre nodi con aggregazione `average`, trasporto deterministico e membership full-mesh iniziale. Il test verifica che il cluster converga senza rete reale entro la finestra operativa dichiarata.

### Criterio di convergenza

Il cluster è considerato convergente quando la distanza assoluta massima tra i valori aggregati osservati sui nodi è:

- `<= 0.05`

La misura è calcolata come:

```text
max(values) - min(values)
```

sui valori correnti del cluster al momento del campionamento.

### Timeout operativo

Il timeout ufficiale del test M09 è:

- `2s`

Motivazione operativa:

- mantiene allineamento con i criteri quantitativi già dichiarati nel README;
- lascia abbastanza round gossip per convergere con ticker da `10ms`;
- resta abbastanza breve da segnalare regressioni reali senza rallentare inutilmente la suite.

### Parametri congelati dal test

Il test usa i seguenti parametri fissi:

- 3 nodi (`node-1`, `node-2`, `node-3`)
- valori iniziali: `10`, `30`, `50`
- aggregazione: `average`
- intervallo round gossip: `10ms`
- soglia di convergenza: `0.05`
- timeout massimo: `2s`

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
- Il test è progettato per essere deterministico nei parametri di setup e sufficientemente tollerante nel polling della convergenza.
- In caso di failure, l'errore riporta timeout, delta massimo osservato e snapshot dei valori del cluster per facilitare la diagnosi.
