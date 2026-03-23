package integration_test

import (
	"fmt"
	"math"
	"testing"
	"time"
)

const (
	m10ComposeCrashService          = "node1"
	m10ComposeResidualServiceA      = "node2"
	m10ComposeResidualServiceB      = "node3"
	m10ComposeExpectedValue         = 30.0
	m10ComposeResidualExpectedValue = 40.0
	m10ComposeConvergenceBand       = 0.05
	m10ComposeStopTimeout           = 30 * time.Second
	m10ComposeRestartTimeout        = 60 * time.Second
	m10ComposeFaultPollInterval     = 1 * time.Second
	m10ComposeResidualTimeout       = 12 * time.Second
	m10ComposeRejoinTimeout         = 20 * time.Second
	m10ComposeMinimumRejoinDelta    = 0.50
)

// TestNodeCrashAndRestart verifica M10 sul cluster Compose reale usando gli script canonici di bootstrap e fault injection.
func TestNodeCrashAndRestart(t *testing.T) {
	harness := newComposeHarness(t)
	harness.requireDocker()
	harness.up()
	defer harness.tryRunScript(composeShutdownTimeout, "cluster_down.sh")
	harness.waitReady()

	t.Logf("bootstrap cluster automatico con strategia %q", composeBootstrapStrategy)
	t.Logf("parametri M10 Compose: servizio_crash=%s timeout_stop=%s timeout_residuo=%s timeout_rejoin=%s banda=%0.6f",
		m10ComposeCrashService,
		m10ComposeStopTimeout,
		m10ComposeResidualTimeout,
		m10ComposeRejoinTimeout,
		m10ComposeConvergenceBand,
	)

	baselineObservation, baselineMetrics, converged := harness.waitForLiveObservationConvergence(
		m10ComposeExpectedValue,
		m10ComposeConvergenceBand,
		composeConvergenceTimeout,
		composePollInterval,
	)
	if err := ensureComposeObservation(baselineObservation); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot live pre-crash:\n%s", formatClusterObservation(baselineObservation))
	if !converged {
		t.Fatalf("cluster Compose non convergente prima del crash entro %s: %s", composeConvergenceTimeout, formatClusterObservation(baselineObservation))
	}

	harness.runFaultScript(m10ComposeStopTimeout, "stop", m10ComposeCrashService, nil)
	if err := harness.waitForServiceDown(m10ComposeCrashService, m10ComposeStopTimeout, m10ComposeFaultPollInterval); err != nil {
		t.Fatal(err)
	}
	stopSnapshotOutput := harness.collectDebugSnapshot(m10ComposeStopTimeout, m10ComposeCrashService, "after-stop")
	t.Logf("snapshot diagnostico dopo stop:\n%s", stopSnapshotOutput)

	residualServices := []string{m10ComposeResidualServiceA, m10ComposeResidualServiceB}
	residualObservation, residualMetrics, residualActive := harness.waitForResidualActivity(
		residualServices,
		baselineMetrics,
		m10ComposeResidualTimeout,
		m10ComposeFaultPollInterval,
	)
	if err := ensureComposeObservation(residualObservation); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot cluster residuo:\n%s", formatClusterObservation(residualObservation))
	if !residualActive {
		t.Fatalf("cluster residuo non osservabile dopo stop di %s entro %s: %s", m10ComposeCrashService, m10ComposeResidualTimeout, formatClusterObservation(residualObservation))
	}
	for _, service := range residualServices {
		nodeID := composeServiceNodeIDs[service]
		before := baselineMetrics[nodeID]
		after := residualMetrics[nodeID]
		t.Logf("attività residua osservata: service=%s node_id=%s rounds_pre=%d rounds_post=%d estimate=%0.6f ready=%t",
			service,
			nodeID,
			before.Rounds,
			after.Rounds,
			after.Estimate,
			after.Ready,
		)
	}

	harness.runFaultScript(m10ComposeRestartTimeout, "start", m10ComposeCrashService, nil)
	if err := harness.waitForServiceReady(m10ComposeCrashService, m10ComposeRestartTimeout, m10ComposeFaultPollInterval); err != nil {
		t.Fatal(err)
	}
	restartSnapshotOutput := harness.collectDebugSnapshot(m10ComposeRestartTimeout, m10ComposeCrashService, "after-restart")
	t.Logf("snapshot diagnostico dopo restart:\n%s", restartSnapshotOutput)

	finalObservation, finalMetrics, finalConverged := harness.waitForLiveObservationConvergence(
		m10ComposeExpectedValue,
		m10ComposeConvergenceBand,
		m10ComposeRejoinTimeout,
		m10ComposeFaultPollInterval,
	)
	if err := ensureComposeObservation(finalObservation); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot live finale dopo rejoin:\n%s", formatClusterObservation(finalObservation))
	if !finalConverged {
		t.Fatalf("cluster Compose non riconvergente dopo restart di %s entro %s: %s", m10ComposeCrashService, m10ComposeRejoinTimeout, formatClusterObservation(finalObservation))
	}

	restartedMetrics := finalMetrics[composeServiceNodeIDs[m10ComposeCrashService]]
	if !restartedMetrics.Ready {
		t.Fatalf("il nodo riavviato %s non risulta ready nello snapshot finale", m10ComposeCrashService)
	}
	if restartedMetrics.Rounds == 0 {
		t.Fatalf("il nodo riavviato %s non ha ancora completato round gossip osservabili", m10ComposeCrashService)
	}
	if math.Abs(restartedMetrics.Estimate-10.0) < m10ComposeMinimumRejoinDelta {
		t.Fatalf("il nodo riavviato %s non ha mostrato rejoin osservabile: estimate=%0.6f", m10ComposeCrashService, restartedMetrics.Estimate)
	}
	if math.Abs(restartedMetrics.Estimate-m10ComposeResidualExpectedValue) >= math.Abs(10.0-m10ComposeResidualExpectedValue) {
		t.Fatalf("il nodo riavviato %s non si è avvicinato al cluster residuo: estimate=%0.6f target_residuo=%0.6f", m10ComposeCrashService, restartedMetrics.Estimate, m10ComposeResidualExpectedValue)
	}

	for _, service := range composeServices {
		nodeID := composeServiceNodeIDs[service]
		metrics := finalMetrics[nodeID]
		t.Logf("metrica finale: service=%s node_id=%s rounds=%d estimate=%0.6f ready=%t",
			service,
			nodeID,
			metrics.Rounds,
			metrics.Estimate,
			metrics.Ready,
		)
	}

	harness.down()
	shutdownObservation, err := harness.readFinalObservation(m10ComposeExpectedValue)
	if err != nil {
		t.Fatalf("lettura snapshot finale di shutdown M10 Compose: %v", err)
	}
	if err := ensureComposeObservation(shutdownObservation); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot finale di shutdown:\n%s", formatClusterObservation(shutdownObservation))
	if !isClusterConverged(shutdownObservation, m10ComposeConvergenceBand) {
		t.Fatalf("cluster non convergente nel teardown finale M10 Compose: %s", formatClusterObservation(shutdownObservation))
	}
	if math.Abs(shutdownObservation.values[composeServiceNodeIDs[m10ComposeCrashService]]-m10ComposeExpectedValue) > m10ComposeConvergenceBand {
		t.Fatalf("il nodo riavviato non è rientrato nella banda finale: node_id=%s estimate=%0.6f", composeServiceNodeIDs[m10ComposeCrashService], shutdownObservation.values[composeServiceNodeIDs[m10ComposeCrashService]])
	}
	if math.Abs(finalObservation.values[composeServiceNodeIDs[m10ComposeCrashService]]-shutdownObservation.values[composeServiceNodeIDs[m10ComposeCrashService]]) > m10ComposeConvergenceBand {
		t.Fatalf("divergenza tra snapshot live finale e shutdown finale del nodo riavviato: live=%0.6f shutdown=%0.6f", finalObservation.values[composeServiceNodeIDs[m10ComposeCrashService]], shutdownObservation.values[composeServiceNodeIDs[m10ComposeCrashService]])
	}
	for _, service := range residualServices {
		nodeID := composeServiceNodeIDs[service]
		if residualMetrics[nodeID].Rounds <= baselineMetrics[nodeID].Rounds {
			t.Fatalf("nessuna prova di round residui per %s: pre=%d post=%d", nodeID, baselineMetrics[nodeID].Rounds, residualMetrics[nodeID].Rounds)
		}
	}
	for _, nodeID := range []string{"node-1", "node-2", "node-3"} {
		value, ok := shutdownObservation.values[nodeID]
		if !ok {
			t.Fatalf("snapshot finale privo del nodo %s: %s", nodeID, formatClusterObservation(shutdownObservation))
		}
		t.Logf("shutdown finale osservato: %s", fmt.Sprintf("node_id=%s estimate=%0.6f", nodeID, value))
	}
}
