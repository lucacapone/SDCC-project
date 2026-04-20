package integration_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	assertNoLocalAliasMembershipTransitions(t, harness.repoRoot)
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

// assertNoLocalAliasMembershipTransitions fallisce se i log Compose mostrano
// transizioni membership dove peer_id coincide con l'alias locale host:port del nodo.
func assertNoLocalAliasMembershipTransitions(t *testing.T, repoRoot string) {
	t.Helper()

	logPath := filepath.Join(repoRoot, "artifacts", "cluster", "latest-cluster-logs.log")
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("lettura log cluster Compose fallita (%s): %v", logPath, err)
	}

	// Alias locali da bloccare: ogni servizio non deve degradare il proprio endpoint canonico.
	localAliasByService := make(map[string]string, len(composeServices))
	portPattern := regexp.MustCompile(`^node(\d+)$`)
	for _, service := range composeServices {
		matches := portPattern.FindStringSubmatch(service)
		if len(matches) != 2 {
			continue
		}
		localAliasByService[service] = service + ":700" + matches[1]
	}
	violations := make([]string, 0)
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, "event=membership_transition") {
			continue
		}
		for service, alias := range localAliasByService {
			if strings.Contains(line, service+"  |") && strings.Contains(line, "peer_id="+alias) {
				violations = append(violations, strings.TrimSpace(line))
			}
		}
	}

	if len(violations) > 0 {
		t.Fatalf("rilevate transizioni membership su alias locale (self) nei log Compose: %s", strings.Join(violations, " || "))
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

// TestMembershipEntriesRestanoStabiliNelCluster3Nodi verifica che ogni nodo mantenga
// tipicamente 2 entry remote (nodi totali - self) senza alias effimeri.
func TestMembershipEntriesRestanoStabiliNelCluster3Nodi(t *testing.T) {
	network := newIntegrationNetwork()
	nodes, cancel := bootstrapCluster(t, network, m09Aggregation, []float64{10, 30, 50}, m09InMemoryGossipInterval)
	defer cancel()
	defer stopCluster(t, nodes)

	observation, stable := waitForCondition(m09InMemoryTimeout, m09InMemoryPollInterval, func() clusterObservation {
		return observeCluster(nodes, averageOf([]float64{10, 30, 50}))
	}, func(clusterObservation) bool {
		for _, node := range nodes {
			if len(node.engine.Membership.Snapshot()) != 2 {
				return false
			}
		}
		return true
	})

	if !stable {
		t.Fatalf("membership_entries non stabili nel cluster 3 nodi: report=%s", formatClusterObservation(observation))
	}
}
