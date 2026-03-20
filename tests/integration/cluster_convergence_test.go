package integration_test

import (
	"testing"
	"time"
)

const (
	m09NodeCount          = 3
	m09Aggregation        = "average"
	m09GossipInterval     = 10 * time.Millisecond
	m09PollInterval       = 20 * time.Millisecond
	m09BootstrapAllowance = 5 * m09GossipInterval
	m09LocalCIBuffer      = 15 * m09PollInterval
	m09Timeout            = m09BootstrapAllowance + m09LocalCIBuffer
	m09ConvergenceBand    = 0.05
)

// TestClusterConvergence verifica che un cluster a tre nodi converga entro la banda e il timeout ufficiali M09.
func TestClusterConvergence(t *testing.T) {
	initialValues := []float64{10, 30, 50}
	expectedValue := averageOf(initialValues)

	t.Logf("bootstrap cluster automatico con strategia %q", clusterBootstrapStrategy)
	t.Logf("parametri M09: nodi=%d aggregazione=%s gossip_interval=%s poll_interval=%s bootstrap_allowance=%s buffer_locale_ci=%s timeout=%s banda=%0.6f",
		m09NodeCount,
		m09Aggregation,
		m09GossipInterval,
		m09PollInterval,
		m09BootstrapAllowance,
		m09LocalCIBuffer,
		m09Timeout,
		m09ConvergenceBand,
	)

	network := newIntegrationNetwork()
	nodes, cancel := bootstrapCluster(t, network, m09Aggregation, initialValues, m09GossipInterval)
	defer cancel()
	defer stopCluster(t, nodes)

	observation, converged := waitForClusterConvergence(nodes, m09Timeout, m09PollInterval, expectedValue, m09ConvergenceBand)
	t.Logf("report finale convergenza:\n%s", formatClusterObservation(observation))

	if !converged {
		t.Fatalf(
			"cluster non convergente entro %s: banda<=%0.6f report=%s",
			m09Timeout,
			m09ConvergenceBand,
			formatClusterObservation(observation),
		)
	}
}
