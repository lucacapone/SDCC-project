package integration_test

import (
	"testing"
	"time"
)

const (
	scaleNodeCount      = 8
	scaleGossipInterval = 12 * time.Millisecond
	scalePollInterval   = 25 * time.Millisecond
	scaleTimeout        = 2 * time.Second
	scaleBand           = 0.10
)

// TestClusterConvergenceScaleInMemory verifica la convergenza con cluster in-memory più ampio.
// Il test usa bootstrapCluster su 8 nodi per esercitare il fanout gossip su una topologia più densa.
func TestClusterConvergenceScaleInMemory(t *testing.T) {
	initialValues := []float64{10, 20, 30, 40, 50, 60, 70, 80}
	if len(initialValues) != scaleNodeCount {
		t.Fatalf("dataset scala non coerente: attesi %d valori, ottenuti %d", scaleNodeCount, len(initialValues))
	}
	expectedValue := averageOf(initialValues)

	network := newIntegrationNetwork()
	nodes, cancel := bootstrapCluster(t, network, m09Aggregation, initialValues, scaleGossipInterval)
	defer cancel()
	defer stopCluster(t, nodes)

	observation, converged := waitForClusterConvergence(nodes, scaleTimeout, scalePollInterval, expectedValue, scaleBand)
	t.Logf("report finale convergenza scale in-memory (nodi=%d):\n%s", scaleNodeCount, formatClusterObservation(observation))

	if !converged {
		t.Fatalf(
			"cluster scale in-memory non convergente entro %s: banda<=%0.6f report=%s",
			scaleTimeout,
			scaleBand,
			formatClusterObservation(observation),
		)
	}
}
