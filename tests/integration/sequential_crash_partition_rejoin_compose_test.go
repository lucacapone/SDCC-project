package integration_test

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"testing"
	"time"
)

const (
	m10ExtendedExpectedValue      = 30.0
	m10ExtendedConvergenceBand    = 0.08
	m10ExtendedResidualPoll       = 1 * time.Second
	m10ExtendedCrashServiceA      = "node1"
	m10ExtendedCrashServiceB      = "node2"
	m10ExtendedResidualService    = "node3"
	m10ExtendedMembershipExpected = 2.0
)

// m10ExtendedDurations raggruppa timeout/poll scenario in modo configurabile da env.
type m10ExtendedDurations struct {
	ScenarioTimeout      time.Duration
	ResidualAliveTimeout time.Duration
	RejoinTimeout        time.Duration
}

// TestSequentialCrashPartitionAndRejoin verifica crash sequenziale di due nodi, partizione temporanea e rejoin.
func TestSequentialCrashPartitionAndRejoin(t *testing.T) {
	durations := loadM10ExtendedDurations(t)

	harness := newComposeHarness(t)
	harness.requireDocker()
	harness.up()
	defer harness.tryRunScript(composeShutdownTimeout, "cluster_down.sh")
	harness.waitReady()

	baselineObservation, baselineMetrics, converged := harness.waitForLiveObservationConvergence(
		m10ExtendedExpectedValue,
		m10ExtendedConvergenceBand,
		composeConvergenceTimeout,
		composePollInterval,
	)
	if err := ensureComposeObservation(baselineObservation); err != nil {
		t.Fatal(err)
	}
	if !converged {
		t.Fatalf("cluster non convergente nel baseline esteso entro %s: %s", composeConvergenceTimeout, formatClusterObservation(baselineObservation))
	}

	t.Logf("avvio scenario combinato M10 esteso con timeout_scenario=%s timeout_residuo=%s timeout_rejoin=%s",
		durations.ScenarioTimeout,
		durations.ResidualAliveTimeout,
		durations.RejoinTimeout,
	)
	harness.runCombinedFaultScenario(durations.ScenarioTimeout, map[string]string{
		"CRASH_NODE_A":           m10ExtendedCrashServiceA,
		"CRASH_NODE_B":           m10ExtendedCrashServiceB,
		"PARTITION_NODE":         m10ExtendedResidualService,
		"SEQUENTIAL_GAP_SECONDS": "2",
		"PARTITION_SECONDS":      "6",
	})

	residualServiceNodeID := harness.serviceNodeIDs[m10ExtendedResidualService]
	_, _, residualAlive := waitForComposeCondition(
		durations.ResidualAliveTimeout,
		m10ExtendedResidualPoll,
		func() (clusterObservation, map[string]composeNodeMetrics, error) {
			return harness.readLiveObservation(m10ExtendedExpectedValue, []string{m10ExtendedResidualService})
		},
		func(_ clusterObservation, metrics map[string]composeNodeMetrics) bool {
			current := metrics[residualServiceNodeID]
			return current.Ready && current.Rounds > baselineMetrics[residualServiceNodeID].Rounds
		},
	)
	if !residualAlive {
		t.Fatalf("cluster residuo non operativo dopo crash sequenziale+partizione entro %s", durations.ResidualAliveTimeout)
	}

	finalObservation, finalMetrics, rejoined := harness.waitForLiveObservationConvergence(
		m10ExtendedExpectedValue,
		m10ExtendedConvergenceBand,
		durations.RejoinTimeout,
		m10ExtendedResidualPoll,
	)
	if err := ensureComposeObservation(finalObservation); err != nil {
		t.Fatal(err)
	}
	if !rejoined {
		t.Fatalf("cluster non riconvergente dopo recovery/rejoin entro %s: %s", durations.RejoinTimeout, formatClusterObservation(finalObservation))
	}

	for _, service := range harness.composeServices {
		nodeID := harness.serviceNodeIDs[service]
		metrics := finalMetrics[nodeID]
		if !metrics.Ready {
			t.Fatalf("nodo non ready dopo rejoin: service=%s node_id=%s", service, nodeID)
		}
		if metrics.KnownPeers < m10ExtendedMembershipExpected {
			t.Fatalf("membership incompleta dopo rejoin: service=%s node_id=%s known_peers=%0.2f", service, nodeID, metrics.KnownPeers)
		}
		if math.Abs(finalObservation.values[nodeID]-m10ExtendedExpectedValue) > m10ExtendedConvergenceBand {
			t.Fatalf("nodo fuori banda finale: service=%s node_id=%s estimate=%0.6f", service, nodeID, finalObservation.values[nodeID])
		}
		t.Logf("verifica finale nodo: service=%s node_id=%s estimate=%0.6f rounds=%d known_peers=%0.2f",
			service,
			nodeID,
			finalObservation.values[nodeID],
			metrics.Rounds,
			metrics.KnownPeers,
		)
	}
}

// waitForComposeCondition applica polling con deadline a una lettura Compose live con metriche.
func waitForComposeCondition(timeout time.Duration, pollEvery time.Duration, observe func() (clusterObservation, map[string]composeNodeMetrics, error), predicate func(clusterObservation, map[string]composeNodeMetrics) bool) (clusterObservation, map[string]composeNodeMetrics, bool) {
	deadline := time.Now().Add(timeout)
	var lastObservation clusterObservation
	var lastMetrics map[string]composeNodeMetrics

	for time.Now().Before(deadline) {
		observation, metrics, err := observe()
		if err == nil {
			lastObservation = observation
			lastMetrics = metrics
			if predicate(observation, metrics) {
				return observation, metrics, true
			}
		}
		time.Sleep(pollEvery)
	}

	return lastObservation, lastMetrics, false
}

// loadM10ExtendedDurations legge i timeout dello scenario esteso da env con fallback sicuro.
func loadM10ExtendedDurations(t *testing.T) m10ExtendedDurations {
	t.Helper()
	return m10ExtendedDurations{
		ScenarioTimeout:      mustDurationFromEnv(t, "SDCC_M10_EXT_SCENARIO_TIMEOUT", 120*time.Second),
		ResidualAliveTimeout: mustDurationFromEnv(t, "SDCC_M10_EXT_RESIDUAL_TIMEOUT", 20*time.Second),
		RejoinTimeout:        mustDurationFromEnv(t, "SDCC_M10_EXT_REJOIN_TIMEOUT", 35*time.Second),
	}
}

// mustDurationFromEnv parse una durata Go (es. "45s") o un intero in secondi.
func mustDurationFromEnv(t *testing.T, key string, fallback time.Duration) time.Duration {
	t.Helper()
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(raw); err == nil {
		if parsed <= 0 {
			t.Fatalf("valore non positivo per %s: %q", key, raw)
		}
		return parsed
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		t.Fatalf("durata non valida per %s=%q; usare formato Go (es. 30s) o secondi interi", key, raw)
	}
	return time.Duration(seconds) * time.Second
}

// Example_mustDurationFromEnv documenta il formato atteso dei timeout configurabili da shell.
func Example_mustDurationFromEnv() {
	fmt.Println("SDCC_M10_EXT_SCENARIO_TIMEOUT=150s")
	fmt.Println("SDCC_M10_EXT_RESIDUAL_TIMEOUT=25")
	fmt.Println("SDCC_M10_EXT_REJOIN_TIMEOUT=40s")
	// Output:
	// SDCC_M10_EXT_SCENARIO_TIMEOUT=150s
	// SDCC_M10_EXT_RESIDUAL_TIMEOUT=25
	// SDCC_M10_EXT_REJOIN_TIMEOUT=40s
}
