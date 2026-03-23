package integration_test

import (
	"testing"
	"time"
)

const (
	m09NodeCount              = 3
	m09Aggregation            = "average"
	m09InMemoryGossipInterval = 10 * time.Millisecond
	m09InMemoryPollInterval   = 20 * time.Millisecond
	m09InMemoryTimeout        = 350 * time.Millisecond
	m09ComposePollInterval    = composePollInterval
	m09ComposeTimeout         = composeConvergenceTimeout
	m09ConvergenceBand        = 0.05
)

// TestClusterConvergence verifica il cluster locale multi-nodo reale avviato tramite Docker Compose.
func TestClusterConvergence(t *testing.T) {
	initialValues := []float64{10, 30, 50}
	expectedValue := averageOf(initialValues)

	harness := newComposeHarness(t)
	harness.requireDocker()
	harness.up()
	defer harness.tryRunScript(composeShutdownTimeout, "cluster_down.sh")
	harness.waitReady()

	t.Logf("bootstrap cluster automatico con strategia %q", composeBootstrapStrategy)
	t.Logf("parametri M09 Compose: nodi=%d aggregazione=%s timeout_prontezza=%s poll_prontezza=%s timeout_convergenza=%s poll_convergenza=%s banda=%0.6f",
		m09NodeCount,
		m09Aggregation,
		composeReadyTimeout,
		composeReadyPollInterval,
		m09ComposeTimeout,
		m09ComposePollInterval,
		m09ConvergenceBand,
	)

	observation, converged := harness.waitForConvergence(expectedValue, m09ConvergenceBand, m09ComposeTimeout, m09ComposePollInterval)
	if err := ensureComposeObservation(observation); err != nil {
		t.Fatal(err)
	}
	t.Logf("report finale convergenza Compose:\n%s", formatClusterObservation(observation))

	if !converged {
		t.Fatalf(
			"cluster Compose non convergente entro %s: banda<=%0.6f report=%s",
			m09ComposeTimeout,
			m09ConvergenceBand,
			formatClusterObservation(observation),
		)
	}
}

// TestClusterConvergenceInMemory mantiene la suite rapida/deterministica storica per debugging locale.
func TestClusterConvergenceInMemory(t *testing.T) {
	initialValues := []float64{10, 30, 50}
	expectedValue := averageOf(initialValues)

	t.Logf("bootstrap cluster automatico con strategia %q", clusterBootstrapStrategy)
	t.Logf("parametri M09 in-memory: nodi=%d aggregazione=%s gossip_interval=%s poll_interval=%s timeout=%s banda=%0.6f",
		m09NodeCount,
		m09Aggregation,
		m09InMemoryGossipInterval,
		m09InMemoryPollInterval,
		m09InMemoryTimeout,
		m09ConvergenceBand,
	)

	network := newIntegrationNetwork()
	nodes, cancel := bootstrapCluster(t, network, m09Aggregation, initialValues, m09InMemoryGossipInterval)
	defer cancel()
	defer stopCluster(t, nodes)

	observation, converged := waitForClusterConvergence(nodes, m09InMemoryTimeout, m09InMemoryPollInterval, expectedValue, m09ConvergenceBand)
	t.Logf("report finale convergenza in-memory:\n%s", formatClusterObservation(observation))

	if !converged {
		t.Fatalf(
			"cluster in-memory non convergente entro %s: banda<=%0.6f report=%s",
			m09InMemoryTimeout,
			m09ConvergenceBand,
			formatClusterObservation(observation),
		)
	}
}
