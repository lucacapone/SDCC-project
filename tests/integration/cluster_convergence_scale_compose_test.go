package integration_test

import (
	"testing"
	"time"
)

const (
	scaleComposeNodeCount            = 6
	scaleComposeConvergenceBand      = 0.10
	scaleComposeReadyTimeout         = 120 * time.Second
	scaleComposeConvergenceTimeout   = 45 * time.Second
	scaleComposePollInterval         = 2 * time.Second
	scaleComposeProjectName          = "sdcc-scale"
	scaleComposeFile                 = "deploy/docker-compose.scale.yml"
	scaleComposeExpectedMinimumRound = 2
)

var scaleComposeServices = []string{"node1", "node2", "node3", "node4", "node5", "node6"}

// TestClusterConvergenceScaleCompose verifica convergenza e osservabilità round/merge sul cluster Compose a 6 nodi.
func TestClusterConvergenceScaleCompose(t *testing.T) {
	initialValues := []float64{10, 30, 50, 70, 90, 110}
	if len(initialValues) != scaleComposeNodeCount {
		t.Fatalf("dataset scale Compose non coerente: attesi %d valori, ottenuti %d", scaleComposeNodeCount, len(initialValues))
	}
	expectedValue := averageOf(initialValues)

	harness := newComposeHarnessWithOptions(t, composeHarnessOptions{
		composeFile: scaleComposeFile,
		projectName: scaleComposeProjectName,
		services:    scaleComposeServices,
	})
	harness.requireDocker()
	harness.up()
	defer harness.tryRunScript(composeShutdownTimeout, "cluster_down.sh")

	t.Logf("bootstrap cluster automatico con strategia %q", composeBootstrapStrategy)
	t.Logf("parametri scale Compose: compose_file=%s progetto=%s nodi=%d timeout_prontezza=%s timeout_convergenza=%s poll=%s banda=%0.6f",
		scaleComposeFile,
		scaleComposeProjectName,
		scaleComposeNodeCount,
		scaleComposeReadyTimeout,
		scaleComposeConvergenceTimeout,
		scaleComposePollInterval,
		scaleComposeConvergenceBand,
	)

	harness.runScriptWithEnv(
		scaleComposeReadyTimeout+10*time.Second,
		map[string]string{
			"TIMEOUT_SECONDS":       "120",
			"POLL_INTERVAL_SECONDS": "2",
		},
		"cluster_wait_ready.sh",
	)

	observation, metricsByNode, converged := harness.waitForLiveObservationConvergence(
		expectedValue,
		scaleComposeConvergenceBand,
		scaleComposeConvergenceTimeout,
		scaleComposePollInterval,
	)
	if err := ensureComposeObservation(observation); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot live convergenza scale Compose:\n%s", formatClusterObservation(observation))

	if !converged {
		t.Fatalf("cluster scale Compose non convergente entro %s: banda<=%0.6f report=%s", scaleComposeConvergenceTimeout, scaleComposeConvergenceBand, formatClusterObservation(observation))
	}

	for _, service := range harness.composeServices {
		nodeID := harness.serviceNodeIDs[service]
		metrics := metricsByNode[nodeID]
		t.Logf("osservabilità scale Compose: service=%s node_id=%s rounds=%d remote_merges=%d estimate=%0.6f ready=%t",
			service,
			nodeID,
			metrics.Rounds,
			metrics.RemoteMerges,
			metrics.Estimate,
			metrics.Ready,
		)
		if !metrics.Ready {
			t.Fatalf("servizio non ready nello snapshot convergenza: service=%s node_id=%s", service, nodeID)
		}
		if metrics.Rounds < scaleComposeExpectedMinimumRound {
			t.Fatalf("round gossip insufficienti per %s: rounds=%d soglia_minima=%d", nodeID, metrics.Rounds, scaleComposeExpectedMinimumRound)
		}
		if metrics.RemoteMerges == 0 {
			t.Fatalf("assenza merge remoti osservabili per %s", nodeID)
		}
	}

	harness.down()
	shutdownObservation, err := harness.readFinalObservation(expectedValue)
	if err != nil {
		t.Fatalf("lettura snapshot finale scale Compose: %v", err)
	}
	if err := ensureComposeObservation(shutdownObservation); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot finale di shutdown scale Compose:\n%s", formatClusterObservation(shutdownObservation))
	if !isClusterConverged(shutdownObservation, scaleComposeConvergenceBand) {
		t.Fatalf("cluster non convergente nel teardown finale scale Compose: %s", formatClusterObservation(shutdownObservation))
	}
}
