# ROADMAP

## Obiettivi e Definition of Done

### Obiettivo generale
Realizzare una piattaforma di aggregazione dati distribuita gossip-based in Go, senza coordinatore centrale, eseguibile in locale con Docker Compose e pronta per deploy su AWS EC2.

### Deliverable misurabili (DoD di progetto)
1. **Almeno 2 aggregazioni implementate e selezionabili da configurazione esterna** (es. `sum` e `average`) con test di correttezza dedicati.
2. **Configurazione esterna completa** (YAML/JSON + env override) per parametri chiave: gossip interval, peer list/bootstrap, timeout, tipo aggregazione, livello log.
3. **Docker Compose multi-nodo** con almeno 3 nodi avviabili con un solo comando, networking dedicato e healthcheck base.
4. **Test crash/restart** riproducibili che simulano stop di un nodo durante il gossip e verifica di ripresa/convergenza dopo restart.
5. **Osservabilità minima**: logging strutturato (JSON o key-value), metriche minime (round gossip, peers contattati, stima aggregato, tempo convergenza) e endpoint di health.

### Criteri globali di accettazione
- Tutti i microtask M01–M12 completati nell'ordine definito.
- Pipeline locale verde: test unitari + integrazione + scenario crash/restart.
- README aggiornato con demo end-to-end e risultati attesi.
- Note deploy EC2 documentate con prerequisiti, passi e limiti noti.

---

## Microtask numerati

### M01 — Gossip design
- **Obiettivo**: definire protocollo gossip (stato locale, formato messaggio, merge/update, convergenza attesa).
- **File/cartelle coinvolti**: `docs/architecture.md`, `internal/gossip/`, `internal/types/`.
- **Comando di verifica**: `go test ./tests/gossip -run TestMergeRules -count=1`.
- **Done criteria**:
  - specifica messaggi/versioning documentata;
  - regole di merge implementate e testate;
  - nessun riferimento a coordinatore centrale.
- **Rischi/edge cases**:
  - doppie consegne/riordino pacchetti;
  - conflitti versione stato;
  - convergenza lenta con peer instabili.

### M02 — Membership
- **Obiettivo**: implementare gestione membership decentralizzata (join/leave/suspect).
- **File/cartelle coinvolti**: `internal/membership/`, `internal/gossip/`, `docs/architecture.md`.
- **Comando di verifica**: `go test ./tests/membership -run TestJoinLeave -count=1`.
- **Done criteria**:
  - join dinamico da bootstrap funzionante;
  - peer inattivi marcati e rimossi con timeout configurabile;
  - stato membership propagato via gossip;
  - comandi di verifica distinti tra suite membership, suite gossip membership e integrazione runtime.
- **Comandi utili aggiuntivi**:
  - verifica unitaria membership → `go test ./tests/membership -run 'TestJoinLeave|TestTimeoutTransitions|TestPruneRemovesExpiredDeadPeerAndBlocksObsoleteReintroduction' -count=1`;
  - verifica gossip membership → `go test ./tests/gossip -run 'TestMergeMembership|TestRoundSerializzaMembershipConIncarnation' -count=1`;
  - verifica integrazione runtime → `go test ./tests/integration -run TestRuntimeMembershipFailureDetection -count=1`.
- **Rischi/edge cases**:
  - split-brain temporaneo;
  - falsi positivi di failure detection;
  - rientro di nodo con stato obsoleto.

### M03 — Transport astratto
- **Obiettivo**: introdurre layer di trasporto astratto (interfaccia) separato dalla logica gossip.
- **File/cartelle coinvolti**: `internal/transport/`, `internal/gossip/`, `cmd/node/` (entrypoint runtime reale; il precedente riferimento a `internal/node/` era architetturale e non corrisponde al layout attuale del repository).
- **Comando di verifica**: `go test ./tests/transport -run TestTransportContract`.
- **Done criteria**:
  - interfaccia `Transport` definita;
  - almeno un adapter concreto (es. HTTP/UDP);
  - test con fake transport per scenari deterministici.
- **Rischi/edge cases**:
  - leak di dettagli protocollo nel dominio;
  - gestione timeout/retry incoerente;
  - deadlock su code/canali.

### M04 — Aggregazione #1
- **Obiettivo**: implementare la prima aggregazione globale (es. `sum`).
- **File/cartelle coinvolti**: `internal/aggregation/sum/` (algoritmo base), `internal/aggregation/`, `internal/gossip/state.go` (merge gossip distribuito/idempotente), `tests/aggregation/sum/sum_convergence_test.go` (suite canonica di convergenza).
- **Comando di verifica**: `go test ./tests/aggregation/sum -run TestSumConvergence`.
- **Done criteria**:
  - algoritmo base deterministico in `internal/aggregation/sum/`;
  - convergenza distribuita/idempotente verificata nel merge gossip di `internal/gossip/state.go`;
  - convergenza verificata su N nodi dalla suite canonica `tests/aggregation/sum/sum_convergence_test.go`;
  - errore finale entro tolleranza definita (se applicabile).
- **Rischi/edge cases**:
  - overflow numerico;
  - duplicate update;
  - nodi lenti che ritardano stabilizzazione.

### M05 — Aggregazione #2
- **Obiettivo**: implementare seconda aggregazione (es. `average` o `min/max`) compatibile col framework.
- **File/cartelle coinvolti**: `internal/aggregation/average/`, `internal/aggregation/min/`, `internal/aggregation/max/`, `internal/aggregation/`, `internal/gossip/`, `tests/aggregation/average/`, `tests/aggregation/min/`, `tests/aggregation/max/`, `tests/gossip/`.
- **Comandi di verifica**:
  - `go test ./tests/aggregation/average -run TestAverageConvergence -count=1`
  - `go test ./tests/aggregation/min ./tests/aggregation/max -count=1`
  - `go test ./tests/gossip -run TestSumRegressionConNuoveAggregazioni -count=1`
- **Done criteria**:
  - aggregazioni `average`, `min` e `max` selezionabili da config;
  - stessa API della #1;
  - suite canoniche di convergenza nei package reali `tests/aggregation/*` verdi;
  - test di regressione multi-aggregazione verdi.
- **Rischi/edge cases**:
  - divisione per zero;
  - precisione floating point;
  - incompatibilità serializzazione stato.

### M06 — Config + validazione
- **Obiettivo**: supportare configurazione esterna e validazione robusta dei parametri.
- **File/cartelle coinvolti**: `configs/`, `internal/config/`, `cmd/node/`, `docs/configuration.md`, `tests/config/config_test.go`.
- **Suite canonica / entrypoint**: `tests/config/config_test.go` con entrypoint principale `TestValidateConfig`, evitando di trattare `internal/config` come sede della suite di test.
- **Comando di verifica**: `go test ./tests/config -run TestValidateConfig -count=1`.
- **Done criteria**:
  - parsing file + override env documentati;
  - validazioni bloccanti con errori chiari;
  - default sensati per ambiente locale;
  - riferimenti documentali allineati alla suite reale `tests/config/config_test.go`.
- **Rischi/edge cases**:
  - mismatch tipi/config incompleta;
  - valori pericolosi (interval=0);
  - drift fra documentazione e runtime.

### M07 — Compose + networking
- **Obiettivo**: predisporre ambiente Docker Compose multi-nodo con networking riproducibile.
- **File/cartelle coinvolti**: `docker-compose.yml`, `Dockerfile`, `deploy/compose/`, `docs/deployment.md`.
- **Comando di verifica**: `docker compose up -d --build && docker compose ps`.
- **Done criteria**:
  - almeno 3 nodi avviati e raggiungibili;
  - bootstrap/membership funzionanti su rete compose;
  - comandi start/stop/documentazione allineati.
- **Rischi/edge cases**:
  - race condition al bootstrap;
  - DNS/container name non risolti;
  - differenze Docker Engine locali.

### M08 — Test unitari
- **Obiettivo**: copertura unit test su componenti core (merge, membership, config, aggregazioni).
- **File/cartelle coinvolti**: `internal/**/**/*_test.go`, `go.mod`, `go.sum`.
- **Comando di verifica**: `go test ./... -run Test -count=1`.
- **Done criteria**:
  - unit test per ogni modulo core;
  - test deterministici e ripetibili;
  - no flaky test noti.
- **Rischi/edge cases**:
  - dipendenze temporali fragili;
  - finti non realistici;
  - copertura sbilanciata su soli casi felici.

### M09 — Test integrazione / convergenza
- **Obiettivo**: validare convergenza end-to-end su cluster locale multi-nodo.
- **File/cartelle coinvolti**: `tests/integration/`, `scripts/`, `docker-compose.yml`, `docs/testing.md`.
- **Comando di verifica**: `go test ./tests/integration -run TestClusterConvergence -count=1`.
- **Done criteria**:
  - avvio cluster test automatico;
  - convergenza entro tempo massimo definito;
  - report del valore finale per nodo.
- **Rischi/edge cases**:
  - variabilità temporale CI;
  - porte occupate/local env sporco;
  - timeout troppo stretti.

### M10 — Test crash/restart
- **Obiettivo**: testare resilienza a crash e rientro nodo.
- **File/cartelle coinvolti**: `tests/integration/`, `scripts/fault_injection/`, `docs/testing.md`.
- **Comando di verifica**: `go test ./tests/integration -run TestNodeCrashAndRestart -count=1`.
- **Done criteria**:
  - crash di almeno 1 nodo durante gossip;
  - cluster continua ad aggiornare aggregato;
  - nodo riavviato rientra e converge.
- **Rischi/edge cases**:
  - perdita stato non recuperabile;
  - false convergence;
  - tempi di rejoin elevati.

### M11 — Logging / observability
- **Obiettivo**: introdurre osservabilità minima per debugging e misure sperimentali.
- **File/cartelle coinvolti**: `internal/observability/`, `internal/node/`, `docs/observability.md`.
- **Comando di verifica**: `go test ./tests/observability -run TestMetricsExposure`.
- **Done criteria**:
  - log strutturati con campi chiave (node_id, round, peers, estimate);
  - metriche minime esportate (endpoint o stdout structured);
  - health/readiness endpoint disponibili.
- **Rischi/edge cases**:
  - overhead logging eccessivo;
  - cardinalità metriche alta;
  - dati sensibili nei log.

### M12 — README/demo + note deploy EC2
- **Obiettivo**: completare documentazione operativa e deploy target AWS EC2.
- **File/cartelle coinvolti**: `README.md`, `docs/deployment_ec2.md`, `docs/testing.md`, `docs/demo.md`.
- **Comando di verifica**: `rg -n "Quickstart|Demo|EC2|Crash" README.md docs/*.md`.
- **Done criteria**:
  - quickstart end-to-end replicabile;
  - demo con output atteso e criteri di successo;
  - checklist deploy EC2 con prerequisiti/costi/limiti.
- **Rischi/edge cases**:
  - istruzioni non allineate al codice;
  - differenze ambiente locale vs EC2;
  - costi cloud non monitorati.

---

## Ordine di esecuzione e dipendenze (anti-ambiguità)
- Sequenza obbligatoria: **M01 → M02 → M03 → M04 → M05 → M06 → M07 → M08 → M09 → M10 → M11 → M12**.
- Gate di avanzamento:
  - M04 richiede M01+M03;
  - M05 richiede M04;
  - M07 richiede M02+M03+M06;
  - M09 richiede M07+M08;
  - M10 richiede M09;
  - M12 richiede completamento di tutti i precedenti.
- Regola di chiarezza: un microtask è “eseguibile” solo se il suo **Comando di verifica** è lanciabile senza modifiche extra non documentate.
