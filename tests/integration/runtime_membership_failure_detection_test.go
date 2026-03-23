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
