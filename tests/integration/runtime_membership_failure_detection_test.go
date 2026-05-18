package integration_test

import (
	"testing"
	"time"

	"sdcc-project/internal/membership"
)

const (
	runtimeFailureDetectionAggregation     = "average"
	runtimeFailureDetectionGossipInterval  = 10 * time.Millisecond
	runtimeFailureDetectionPollInterval    = 10 * time.Millisecond
	runtimeFailureDetectionSuspectTimeout  = 30 * time.Millisecond
	runtimeFailureDetectionDeadTimeout     = 70 * time.Millisecond
	runtimeFailureDetectionSuspectDeadline = 180 * time.Millisecond
	runtimeFailureDetectionDeadDeadline    = 280 * time.Millisecond
)

// TestRuntimeMembershipFailureDetection verifica che il loop runtime degradi automaticamente
// un peer inattivo fino a suspect e poi dead senza invocazioni manuali dei test.
func TestRuntimeMembershipFailureDetection(t *testing.T) {
	network := newIntegrationNetwork()
	nodes, cancel := bootstrapCluster(t, network, runtimeFailureDetectionAggregation, []float64{10, 30, 50}, runtimeFailureDetectionGossipInterval)
	defer cancel()
	defer stopCluster(t, nodes)

	clusterAddresses := make([]string, 0, len(nodes))
	for _, node := range nodes {
		clusterAddresses = append(clusterAddresses, node.address)
	}

	applyMembershipTimeouts(nodes, clusterAddresses, membership.Config{
		SuspectTimeout: runtimeFailureDetectionSuspectTimeout,
		DeadTimeout:    runtimeFailureDetectionDeadTimeout,
	})

	crashedNodeIndex := 2
	crashedNodeID := nodes[crashedNodeIndex].address
	observer := nodes[0]

	if err := nodes[crashedNodeIndex].engine.Stop(); err != nil {
		t.Fatalf("stop nodo inattivo %s: %v", crashedNodeID, err)
	}
	nodes[crashedNodeIndex] = nil

	suspectPeer, suspectObserved := waitForMembershipStatus(
		observer,
		crashedNodeID,
		membership.Suspect,
		runtimeFailureDetectionSuspectDeadline,
		runtimeFailureDetectionPollInterval,
	)
	if !suspectObserved {
		t.Fatalf("peer inattivo non marcato suspect entro %s: peer=%+v snapshot=%v", runtimeFailureDetectionSuspectDeadline, suspectPeer, observer.engine.Membership.Snapshot())
	}
	if suspectPeer.Status != membership.Suspect {
		t.Fatalf("atteso stato suspect per %s, got=%s", crashedNodeID, suspectPeer.Status)
	}
	if time.Since(suspectPeer.LastSeen) < runtimeFailureDetectionSuspectTimeout {
		t.Fatalf("last_seen troppo recente per la transizione suspect: elapsed=%s soglia=%s", time.Since(suspectPeer.LastSeen), runtimeFailureDetectionSuspectTimeout)
	}

	deadPeer, deadObserved := waitForMembershipStatus(
		observer,
		crashedNodeID,
		membership.Dead,
		runtimeFailureDetectionDeadDeadline,
		runtimeFailureDetectionPollInterval,
	)
	if !deadObserved {
		t.Fatalf("peer inattivo non marcato dead entro %s: peer=%+v snapshot=%v", runtimeFailureDetectionDeadDeadline, deadPeer, observer.engine.Membership.Snapshot())
	}
	if deadPeer.Status != membership.Dead {
		t.Fatalf("atteso stato dead per %s, got=%s", crashedNodeID, deadPeer.Status)
	}
	if deadPeer.LastSeen.Before(suspectPeer.LastSeen) {
		t.Fatalf("last_seen regressivo durante il degrado runtime: suspect=%s dead=%s", suspectPeer.LastSeen, deadPeer.LastSeen)
	}
}

const (
	runtimeStableAggregation       = "average"
	runtimeStableGossipInterval    = 20 * time.Millisecond
	runtimeStablePollInterval      = 20 * time.Millisecond
	runtimeStableBootstrapTimeout  = 600 * time.Millisecond
	runtimeStableObservationWindow = 600 * time.Millisecond
	runtimeStableSuspectTimeout    = 180 * time.Millisecond
	runtimeStableDeadTimeout       = 360 * time.Millisecond
	runtimeStableConvergenceBand   = 0.05
)

// TestRuntimeMembershipStableLongRunNoFalseSuspectConvergence documenta una run
// più lunga con parametri failure-detection meno aggressivi: il timeout suspect
// resta molto sopra il gap massimo atteso tra heartbeat gossip diretti e la
// convergenza average rimane stabile senza peer falsamente sospetti.
func TestRuntimeMembershipStableLongRunNoFalseSuspectConvergence(t *testing.T) {
	initialValues := []float64{10, 30, 50, 70, 90, 110}
	expectedValue := averageOf(initialValues)
	network := newIntegrationNetwork()
	nodes, cancel := bootstrapClusterWithMembershipConfig(
		t,
		network,
		runtimeStableAggregation,
		initialValues,
		runtimeStableGossipInterval,
		membership.Config{
			SuspectTimeout: runtimeStableSuspectTimeout,
			DeadTimeout:    runtimeStableDeadTimeout,
		},
	)
	defer cancel()
	defer stopCluster(t, nodes)

	// La run usa fanout full-mesh per rendere la frequenza di heartbeat/merge
	// diretta e verificabile: ogni peer è target a ogni round gossip.
	for _, node := range nodes {
		node.engine.Fanout = len(nodes) - 1
	}

	expectedReceiveGap := runtimeStableGossipInterval
	if runtimeStableSuspectTimeout <= expectedReceiveGap {
		t.Fatalf("parametri prova non conservativi: suspect=%s gap_atteso=%s", runtimeStableSuspectTimeout, expectedReceiveGap)
	}
	t.Logf(
		"prova long-run failure detection: nodi=%d gossip_interval=%s fanout=%d gap_atteso_peer=%s suspect_timeout=%s dead_timeout=%s bootstrap_timeout=%s finestra_stabile=%s banda=%0.6f",
		len(nodes),
		runtimeStableGossipInterval,
		len(nodes)-1,
		expectedReceiveGap,
		runtimeStableSuspectTimeout,
		runtimeStableDeadTimeout,
		runtimeStableBootstrapTimeout,
		runtimeStableObservationWindow,
		runtimeStableConvergenceBand,
	)

	bootstrapObservation, bootstrapped := waitForAliveConvergence(
		nodes,
		runtimeStableBootstrapTimeout,
		runtimeStablePollInterval,
		expectedValue,
		runtimeStableConvergenceBand,
	)
	if !bootstrapped {
		t.Fatalf("cluster non convergente senza suspect entro il bootstrap long-run: %s", formatClusterObservation(bootstrapObservation))
	}

	observation, stable := waitForStableAliveConvergence(
		nodes,
		runtimeStableObservationWindow,
		runtimeStablePollInterval,
		expectedValue,
		runtimeStableConvergenceBand,
	)
	t.Logf("report finale long-run failure detection:\n%s", formatClusterObservation(observation))
	if !stable {
		t.Fatalf("convergenza non stabile o falsi suspect osservati nella run lunga: %s", formatClusterObservation(observation))
	}
}

// waitForAliveConvergence attende il primo snapshot convergente che non contenga
// peer sospetti, così la finestra stabile parte da uno stato già valido.
func waitForAliveConvergence(nodes []*clusterNode, timeout time.Duration, pollEvery time.Duration, expectedValue float64, threshold float64) (clusterObservation, bool) {
	return waitForCondition(timeout, pollEvery, func() clusterObservation {
		return observeCluster(nodes, expectedValue)
	}, func(observation clusterObservation) bool {
		return isClusterConverged(observation, threshold) && allMembershipPeersAlive(nodes)
	})
}

// waitForStableAliveConvergence richiede che, per l'intera finestra osservata,
// il cluster resti convergente e ogni snapshot membership contenga solo peer alive.
func waitForStableAliveConvergence(nodes []*clusterNode, window time.Duration, pollEvery time.Duration, expectedValue float64, threshold float64) (clusterObservation, bool) {
	lastObservation := observeCluster(nodes, expectedValue)
	if !isClusterConverged(lastObservation, threshold) || !allMembershipPeersAlive(nodes) {
		return lastObservation, false
	}

	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	deadline := time.NewTimer(window)
	defer deadline.Stop()

	for {
		select {
		case <-ticker.C:
			lastObservation = observeCluster(nodes, expectedValue)
			if !isClusterConverged(lastObservation, threshold) || !allMembershipPeersAlive(nodes) {
				return lastObservation, false
			}
		case <-deadline.C:
			lastObservation = observeCluster(nodes, expectedValue)
			return lastObservation, isClusterConverged(lastObservation, threshold) && allMembershipPeersAlive(nodes)
		}
	}
}

// allMembershipPeersAlive controlla che la failure detection non abbia prodotto
// stati suspect/dead/leave su peer ancora attivi nella rete di test.
func allMembershipPeersAlive(nodes []*clusterNode) bool {
	for _, node := range nodes {
		if node == nil || node.engine == nil || node.engine.Membership == nil {
			return false
		}
		for _, peer := range node.engine.Membership.Snapshot() {
			if peer.Status != membership.Alive {
				return false
			}
		}
	}
	return true
}

// applyMembershipTimeouts sostituisce in modo esplicito la configurazione timeout dei nodi di test.
func applyMembershipTimeouts(nodes []*clusterNode, addresses []string, cfg membership.Config) {
	for _, node := range nodes {
		if node == nil {
			continue
		}
		node.engine.Membership = fullMeshMembershipWithConfig(node.address, addresses, cfg)
	}
}

// fullMeshMembershipWithConfig costruisce una membership iniziale full-mesh con timeout osservabili ridotti.
func fullMeshMembershipWithConfig(self string, addresses []string, cfg membership.Config) *membership.Set {
	set := membership.NewSetWithConfig(cfg)
	now := time.Now().UTC()
	for _, address := range addresses {
		if address == self {
			continue
		}
		set.Join(address, now)
	}
	return set
}

// waitForMembershipStatus osserva la membership runtime del nodo fino al raggiungimento dello stato atteso.
func waitForMembershipStatus(node *clusterNode, peerID string, expected membership.Status, timeout time.Duration, pollEvery time.Duration) (membership.Peer, bool) {
	var observed membership.Peer
	_, ok := waitForCondition(timeout, pollEvery, func() clusterObservation {
		peer, exists := snapshotMembershipPeer(node, peerID)
		if exists {
			observed = peer
		}
		return clusterObservation{}
	}, func(clusterObservation) bool {
		peer, exists := snapshotMembershipPeer(node, peerID)
		if !exists {
			return false
		}
		observed = peer
		return peer.Status == expected
	})
	return observed, ok
}

// snapshotMembershipPeer estrae un peer specifico dallo snapshot corrente della membership.
func snapshotMembershipPeer(node *clusterNode, peerID string) (membership.Peer, bool) {
	for _, peer := range node.engine.Membership.Snapshot() {
		if peer.NodeID == peerID {
			return peer, true
		}
	}
	return membership.Peer{}, false
}
