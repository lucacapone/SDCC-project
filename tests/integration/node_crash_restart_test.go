package integration_test

import (
	"fmt"
	"math"
	"testing"
	"time"
)

const (
	crashRestartNodeCount             = 3
	crashRestartAggregation           = "average"
	crashRestartGossipInterval        = 10 * time.Millisecond
	crashRestartPollInterval          = 20 * time.Millisecond
	crashRestartBootstrapTimeout      = 120 * time.Millisecond
	crashRestartCrashTimeout          = 220 * time.Millisecond
	crashRestartRejoinTimeout         = 320 * time.Millisecond
	crashRestartConvergenceBand       = 0.08
	crashRestartResidualExpectedBand  = 0.05
	crashRestartStabilityPolls        = 3
	crashRestartResidualSnapshotCount = 3
	crashRestartRestartValueOffset    = 17.0
	crashRestartMinimumRejoinDelta    = 0.50
)

// TestNodeCrashAndRestartInMemory mantiene la variante rapida/deterministica del vecchio scenario M10 per debugging locale.
func TestNodeCrashAndRestartInMemory(t *testing.T) {
	initialValues := []float64{10, 30, 90}
	scenarioReferenceValue := averageOf(initialValues)
	crashedNodeIndex := 0
	crashedNodeID := fmt.Sprintf("node-%d", crashedNodeIndex+1)
	allAddresses := []string{"node-1", "node-2", "node-3"}

	// `average` è una buona aggregazione osservabile per il rejoin perché il nodo rientrato deve riallineare
	// il proprio contributo verso una banda comune, evitando di restare inchiodato al valore di restart.
	t.Logf("bootstrap cluster automatico con strategia %q", clusterBootstrapStrategy)
	t.Logf("parametri crash/restart: nodi=%d aggregazione=%s gossip_interval=%s poll_interval=%s bootstrap_timeout=%s crash_timeout=%s rejoin_timeout=%s banda_rejoin=%0.6f banda_residua=%0.6f poll_stabili=%d",
		crashRestartNodeCount,
		crashRestartAggregation,
		crashRestartGossipInterval,
		crashRestartPollInterval,
		crashRestartBootstrapTimeout,
		crashRestartCrashTimeout,
		crashRestartRejoinTimeout,
		crashRestartConvergenceBand,
		crashRestartResidualExpectedBand,
		crashRestartStabilityPolls,
	)

	network := newIntegrationNetwork()
	nodes, cancel := bootstrapCluster(t, network, crashRestartAggregation, initialValues, crashRestartGossipInterval)
	defer cancel()
	defer stopCluster(t, nodes)

	initialObservation := observeCluster(nodes, scenarioReferenceValue)
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
	t.Logf("nodo crashato deregistrato correttamente dal transport di test: node_id=%s", crashedNodeID)

	residualNodes := []*clusterNode{nodes[1], nodes[2]}
	residualObservation, residualConverged := waitForClusterConvergence(residualNodes, crashRestartCrashTimeout, crashRestartPollInterval, scenarioReferenceValue, crashRestartResidualExpectedBand)
	t.Logf("valori del cluster residuo: %s", formatClusterObservation(residualObservation))
	if !residualConverged {
		t.Fatalf("cluster residuo non convergente dopo crash del nodo %s: %s", crashedNodeID, formatClusterObservation(residualObservation))
	}

	residualSnapshots, residualStable := collectStableConvergenceSnapshots(
		residualNodes,
		crashRestartCrashTimeout,
		crashRestartPollInterval,
		scenarioReferenceValue,
		crashRestartResidualExpectedBand,
		crashRestartResidualSnapshotCount,
	)
	if !residualStable {
		t.Fatalf("cluster residuo non stabilizzato su %d poll consecutivi: %s", crashRestartResidualSnapshotCount, formatObservationSequence(residualSnapshots))
	}
	if !residualSnapshotsShowCoherentProgress(residualSnapshots, crashRestartResidualExpectedBand) {
		t.Fatalf("gli snapshot del cluster residuo non mostrano progresso/stabilizzazione coerente: %s", formatObservationSequence(residualSnapshots))
	}
	t.Logf("snapshot coerenti del cluster residuo: %s", formatObservationSequence(residualSnapshots))
	residualConsensusValue := averageOf(observationValues(residualSnapshots[len(residualSnapshots)-1]))
	t.Logf("valore informativo di rejoin derivato dal cluster residuo stabile: %0.6f", residualConsensusValue)

	restartInitialValue := initialValues[crashedNodeIndex] + crashRestartRestartValueOffset
	restartedNode := restartClusterNode(t, network, crashedNodeID, crashRestartAggregation, restartInitialValue, allAddresses, crashRestartGossipInterval)
	nodes[crashedNodeIndex] = restartedNode
	if !network.isRegistered(crashedNodeID) {
		t.Fatalf("il nodo riavviato %s non risulta registrato sulla rete di test", crashedNodeID)
	}

	afterRestartObservation, restartedObservedUpdate := waitForCondition(
		crashRestartRejoinTimeout,
		crashRestartPollInterval,
		func() clusterObservation { return observeCluster(nodes, scenarioReferenceValue) },
		func(observation clusterObservation) bool {
			restartedValue := observation.values[crashedNodeID]
			return math.Abs(restartedValue-restartInitialValue) >= crashRestartMinimumRejoinDelta
		},
	)
	t.Logf("valori dopo il restart: %s", formatClusterObservation(afterRestartObservation))
	if !restartedObservedUpdate {
		t.Fatalf("il nodo riavviato %s non ha mostrato un rejoin reale entro %s: valore_restart=%0.6f snapshot=%s", crashedNodeID, crashRestartRejoinTimeout, restartInitialValue, formatClusterObservation(afterRestartObservation))
	}
	if math.Abs(afterRestartObservation.values[crashedNodeID]-restartInitialValue) < crashRestartMinimumRejoinDelta {
		t.Fatalf("il nodo riavviato %s è rimasto troppo vicino al valore di restart %0.6f", crashedNodeID, restartInitialValue)
	}

	finalSnapshots, fullyStable := collectStableConvergenceSnapshots(
		nodes,
		crashRestartRejoinTimeout,
		crashRestartPollInterval,
		residualConsensusValue,
		crashRestartConvergenceBand,
		crashRestartStabilityPolls,
	)
	if !fullyStable {
		t.Fatalf("cluster non stabilizzato dopo rejoin del nodo %s: %s", crashedNodeID, formatObservationSequence(finalSnapshots))
	}
	finalObservation := finalSnapshots[len(finalSnapshots)-1]
	bandDistance := distanceFromClusterBand(finalObservation, crashedNodeID)
	informativeDistance := math.Abs(finalObservation.values[crashedNodeID] - residualConsensusValue)
	restartInformativeDistance := math.Abs(restartInitialValue - residualConsensusValue)
	t.Logf("valore finale del nodo rientrato: node_id=%s value=%0.6f delta_da_pre_crash=%0.6f delta_da_banda_cluster=%0.6f delta_da_atteso=%0.6f delta_restart_da_atteso=%0.6f",
		crashedNodeID,
		finalObservation.values[crashedNodeID],
		math.Abs(finalObservation.values[crashedNodeID]-valueBeforeCrash),
		bandDistance,
		informativeDistance,
		restartInformativeDistance,
	)
	t.Logf("snapshot finali stabili: %s", formatObservationSequence(finalSnapshots))
	if finalObservation.maxDelta > crashRestartConvergenceBand {
		t.Fatalf("banda finale del cluster oltre soglia dopo rejoin del nodo %s: banda=%0.6f soglia=%0.6f", crashedNodeID, finalObservation.maxDelta, crashRestartConvergenceBand)
	}
	if bandDistance > crashRestartConvergenceBand {
		t.Fatalf("il nodo rientrato %s è fuori dalla banda del cluster: distanza=%0.6f banda=%0.6f snapshot=%s", crashedNodeID, bandDistance, crashRestartConvergenceBand, formatClusterObservation(finalObservation))
	}
	if informativeDistance >= restartInformativeDistance {
		t.Fatalf("il nodo rientrato %s non si è avvicinato al valore atteso informativo %0.6f: distanza_finale=%0.6f distanza_restart=%0.6f snapshot=%s", crashedNodeID, residualConsensusValue, informativeDistance, restartInformativeDistance, formatClusterObservation(finalObservation))
	}
	if math.Abs(finalObservation.values[crashedNodeID]-restartInitialValue) < crashRestartMinimumRejoinDelta {
		t.Fatalf("il nodo rientrato %s è ancora troppo vicino al valore di restart %0.6f nel report finale", crashedNodeID, restartInitialValue)
	}
}

// collectStableConvergenceSnapshots richiede che la convergenza sia soddisfatta per più poll consecutivi, non per un singolo snapshot fortunato.
func collectStableConvergenceSnapshots(nodes []*clusterNode, timeout time.Duration, pollEvery time.Duration, expectedValue float64, threshold float64, requiredSnapshots int) ([]clusterObservation, bool) {
	observations := make([]clusterObservation, 0, requiredSnapshots)

	observation, satisfied := waitForCondition(timeout, pollEvery, func() clusterObservation {
		return observeCluster(nodes, expectedValue)
	}, func(observation clusterObservation) bool {
		if !isClusterConverged(observation, threshold) {
			observations = observations[:0]
			return false
		}
		observations = append(observations, observation)
		if len(observations) > requiredSnapshots {
			observations = observations[len(observations)-requiredSnapshots:]
		}
		return len(observations) >= requiredSnapshots
	})
	if !satisfied {
		if len(observations) == 0 {
			observations = append(observations, observation)
		} else if !sameObservation(observations[len(observations)-1], observation) {
			observations = append(observations, observation)
		}
	}
	return observations, satisfied
}

// residualSnapshotsShowCoherentProgress accetta sia un miglioramento monotono della banda sia una stabilizzazione entro soglia.
func residualSnapshotsShowCoherentProgress(observations []clusterObservation, threshold float64) bool {
	if len(observations) < 2 {
		return false
	}

	for index := 1; index < len(observations); index++ {
		previous := observations[index-1]
		current := observations[index]
		progressed := current.maxDelta <= previous.maxDelta+1e-9
		stabilized := previous.maxDelta <= threshold && current.maxDelta <= threshold
		if !progressed && !stabilized {
			return false
		}
	}
	return true
}

// distanceFromClusterBand misura quanto il nodo osservato si discosta dal resto del cluster nello snapshot finale.
func distanceFromClusterBand(observation clusterObservation, nodeID string) float64 {
	nodeValue := observation.values[nodeID]
	maxDistance := 0.0
	for currentNodeID, currentValue := range observation.values {
		if currentNodeID == nodeID {
			continue
		}
		distance := math.Abs(nodeValue - currentValue)
		if distance > maxDistance {
			maxDistance = distance
		}
	}
	return maxDistance
}

// formatObservationSequence rende leggibile una piccola finestra di snapshot consecutivi del test.
func formatObservationSequence(observations []clusterObservation) string {
	if len(observations) == 0 {
		return "nessuno snapshot"
	}

	formatted := make([]string, 0, len(observations))
	for index, observation := range observations {
		formatted = append(formatted, fmt.Sprintf("snapshot_%d={%s}", index+1, formatClusterObservation(observation)))
	}
	return joinObservations(formatted)
}

// joinObservations centralizza il formato compatto degli snapshot consecutivi nei log di test.
func joinObservations(parts []string) string {
	result := ""
	for index, part := range parts {
		if index > 0 {
			result += " | "
		}
		result += part
	}
	return result
}

// sameObservation evita duplicazioni inutili quando il timeout scade sullo stesso snapshot già registrato.
func sameObservation(left clusterObservation, right clusterObservation) bool {
	if len(left.values) != len(right.values) {
		return false
	}
	if math.Abs(left.maxDelta-right.maxDelta) > 1e-9 || math.Abs(left.referenceMaxOffset-right.referenceMaxOffset) > 1e-9 {
		return false
	}
	for nodeID, leftValue := range left.values {
		rightValue, ok := right.values[nodeID]
		if !ok || math.Abs(leftValue-rightValue) > 1e-9 {
			return false
		}
	}
	return true
}

// observationValues estrae i valori numerici di uno snapshot per calcolare riferimenti informativi derivati dal cluster.
func observationValues(observation clusterObservation) []float64 {
	values := make([]float64, 0, len(observation.values))
	for _, value := range observation.values {
		values = append(values, value)
	}
	return values
}
