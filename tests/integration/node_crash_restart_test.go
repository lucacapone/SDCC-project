package integration_test

import (
	"fmt"
	"math"
	"testing"
	"time"
)

const (
	crashRestartNodeCount           = 3
	crashRestartAggregation         = "average"
	crashRestartGossipInterval      = 10 * time.Millisecond
	crashRestartPollInterval        = 20 * time.Millisecond
	crashRestartBootstrapTimeout    = 120 * time.Millisecond
	crashRestartCrashTimeout        = 220 * time.Millisecond
	crashRestartRejoinTimeout       = 320 * time.Millisecond
	crashRestartConvergenceBand     = 0.08
	crashRestartStabilizationWindow = 40 * time.Millisecond
	crashRestartRestartValueOffset  = 17.0
)

// TestNodeCrashAndRestart verifica crash, convergenza residua, restart e rejoin di un nodo nel cluster in-memory canonico.
func TestNodeCrashAndRestart(t *testing.T) {
	initialValues := []float64{10, 30, 90}
	expectedValue := averageOf(initialValues)
	crashedNodeIndex := 0
	crashedNodeID := fmt.Sprintf("node-%d", crashedNodeIndex+1)
	allAddresses := []string{"node-1", "node-2", "node-3"}

	t.Logf("bootstrap cluster automatico con strategia %q", clusterBootstrapStrategy)
	t.Logf("parametri crash/restart: nodi=%d aggregazione=%s gossip_interval=%s poll_interval=%s bootstrap_timeout=%s crash_timeout=%s rejoin_timeout=%s banda=%0.6f stabilizzazione=%s",
		crashRestartNodeCount,
		crashRestartAggregation,
		crashRestartGossipInterval,
		crashRestartPollInterval,
		crashRestartBootstrapTimeout,
		crashRestartCrashTimeout,
		crashRestartRejoinTimeout,
		crashRestartConvergenceBand,
		crashRestartStabilizationWindow,
	)

	network := newIntegrationNetwork()
	nodes, cancel := bootstrapCluster(t, network, crashRestartAggregation, initialValues, crashRestartGossipInterval)
	defer cancel()
	defer stopCluster(t, nodes)

	initialObservation := observeCluster(nodes, expectedValue)
	activityObservation, active := waitForClusterActivity(nodes, crashRestartBootstrapTimeout, crashRestartPollInterval, initialObservation)
	if !active {
		t.Fatalf("nessuna attività gossip osservabile prima del crash: snapshot_iniziale=%s snapshot_finale=%s", formatClusterObservation(initialObservation), formatClusterObservation(activityObservation))
	}
	t.Logf("valori per nodo prima del crash: %s", formatClusterObservation(activityObservation))

	crashedNode := nodes[crashedNodeIndex]
	valueBeforeCrash := crashedNode.engine.State.Value
	if err := crashedNode.engine.Stop(); err != nil {
		t.Fatalf("crash nodo %s: %v", crashedNode.address, err)
	}
	nodes[crashedNodeIndex] = nil
	if network.isRegistered(crashedNodeID) {
		t.Fatalf("il nodo crashato %s risulta ancora registrato sulla rete di test", crashedNodeID)
	}

	residualNodes := []*clusterNode{nodes[1], nodes[2]}
	residualObservation, residualConverged := waitForClusterConvergence(residualNodes, crashRestartCrashTimeout, crashRestartPollInterval, expectedValue, crashRestartConvergenceBand)
	t.Logf("valori del cluster residuo: %s", formatClusterObservation(residualObservation))
	if !residualConverged {
		t.Fatalf("cluster residuo non convergente dopo crash del nodo %s: %s", crashedNodeID, formatClusterObservation(residualObservation))
	}
	if residualObservation.maxDelta > crashRestartConvergenceBand {
		t.Fatalf("banda cluster residuo oltre soglia dopo crash del nodo %s: banda=%0.6f soglia=%0.6f", crashedNodeID, residualObservation.maxDelta, crashRestartConvergenceBand)
	}

	restartInitialValue := initialValues[crashedNodeIndex] + crashRestartRestartValueOffset
	restartedNode := restartClusterNode(t, network, crashedNodeID, crashRestartAggregation, restartInitialValue, allAddresses, crashRestartGossipInterval)
	nodes[crashedNodeIndex] = restartedNode
	if !network.isRegistered(crashedNodeID) {
		t.Fatalf("il nodo riavviato %s non risulta registrato sulla rete di test", crashedNodeID)
	}

	afterRestartObservation, restartedObservedUpdate := waitForCondition(
		crashRestartRejoinTimeout,
		crashRestartPollInterval,
		func() clusterObservation { return observeCluster(nodes, expectedValue) },
		func(observation clusterObservation) bool {
			restartedValue := observation.values[crashedNodeID]
			return math.Abs(restartedValue-restartInitialValue) > 0
		},
	)
	t.Logf("valori dopo il restart: %s", formatClusterObservation(afterRestartObservation))
	if !restartedObservedUpdate {
		t.Fatalf("il nodo riavviato %s non ha ricevuto aggiornamenti entro %s: valore_restart=%0.6f snapshot=%s", crashedNodeID, crashRestartRejoinTimeout, restartInitialValue, formatClusterObservation(afterRestartObservation))
	}
	if math.Abs(afterRestartObservation.values[crashedNodeID]-restartInitialValue) == 0 {
		t.Fatalf("il nodo riavviato %s è rimasto sul valore di restart %0.6f", crashedNodeID, restartInitialValue)
	}

	finalObservation, fullyConverged := waitForClusterConvergence(nodes, crashRestartRejoinTimeout, crashRestartPollInterval, expectedValue, crashRestartConvergenceBand)
	t.Logf("valore finale del nodo rientrato: node_id=%s value=%0.6f delta_da_pre_crash=%0.6f", crashedNodeID, finalObservation.values[crashedNodeID], math.Abs(finalObservation.values[crashedNodeID]-valueBeforeCrash))
	t.Logf("banda finale del cluster: %0.6f", finalObservation.maxDelta)
	if !fullyConverged {
		t.Fatalf("cluster non convergente dopo restart del nodo %s: %s", crashedNodeID, formatClusterObservation(finalObservation))
	}
	if finalObservation.maxDelta > crashRestartConvergenceBand {
		t.Fatalf("banda finale del cluster oltre soglia dopo rejoin del nodo %s: banda=%0.6f soglia=%0.6f", crashedNodeID, finalObservation.maxDelta, crashRestartConvergenceBand)
	}
	if math.Abs(finalObservation.values[crashedNodeID]-restartInitialValue) == 0 {
		t.Fatalf("il nodo rientrato %s è ancora al valore di restart %0.6f nel report finale", crashedNodeID, restartInitialValue)
	}

	stabilizedObservation, stable := waitForStableClusterConvergence(nodes, crashRestartStabilizationWindow, crashRestartPollInterval, expectedValue, crashRestartConvergenceBand)
	if !stable {
		t.Fatalf("cluster non stabile dopo finestra di stabilizzazione %s: %s", crashRestartStabilizationWindow, formatClusterObservation(stabilizedObservation))
	}
}
