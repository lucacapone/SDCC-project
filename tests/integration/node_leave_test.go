package integration_test

import (
	"context"
	"testing"
	"time"

	"sdcc-project/internal/membership"
)

const (
	leaveAggregation        = "average"
	leaveGossipInterval     = 10 * time.Millisecond
	leavePollInterval       = 10 * time.Millisecond
	leaveConvergenceTimeout = 300 * time.Millisecond
	leaveConvergenceBand    = 0.08

	leaveSuspectTimeout  = 30 * time.Millisecond
	leaveDeadTimeout     = 60 * time.Millisecond
	leavePruneRetention  = 80 * time.Millisecond
	leavePropagationWait = 200 * time.Millisecond
	leavePruneDeadline   = 400 * time.Millisecond
	leaveIdleWindow      = 130 * time.Millisecond
)

// TestVoluntaryLeaveMaintainsResidualConvergence verifica che un nodo in uscita volontaria
// invii un annuncio leave e che il cluster residuo continui a convergere.
func TestVoluntaryLeaveMaintainsResidualConvergence(t *testing.T) {
	initialValues := []float64{10, 30, 50}
	expectedResidual := averageOf([]float64{30, 50})

	network := newIntegrationNetwork()
	nodes, cancel := bootstrapCluster(t, network, leaveAggregation, initialValues, leaveGossipInterval)
	defer cancel()
	defer stopCluster(t, nodes)

	leaving := nodes[0]
	if err := leaving.engine.AnnounceLeave(context.Background()); err != nil {
		t.Fatalf("annuncio leave fallito: %v", err)
	}
	if err := leaving.engine.Stop(); err != nil {
		t.Fatalf("stop nodo in leave fallito: %v", err)
	}
	nodes[0] = nil

	residualNodes := []*clusterNode{nodes[1], nodes[2]}
	observation, converged := waitForClusterConvergence(
		residualNodes,
		leaveConvergenceTimeout,
		leavePollInterval,
		expectedResidual,
		leaveConvergenceBand,
	)
	if !converged {
		t.Fatalf("cluster residuo non convergente dopo leave volontario: %s", formatClusterObservation(observation))
	}

	for _, observer := range residualNodes {
		peer, ok := waitForMembershipStatus(observer, leaving.address, membership.Left, leavePropagationWait, leavePollInterval)
		if !ok {
			t.Fatalf("nodo %s non ha osservato lo stato leave di %s entro %s", observer.address, leaving.address, leavePropagationWait)
		}
		if peer.Incarnation == 0 {
			t.Fatalf("incarnation leave non incrementata per %s nel nodo %s: %+v", leaving.address, observer.address, peer)
		}
	}
}

// TestVoluntaryLeaveNodeNotTargetAfterProtocolWindow verifica che, dopo propagate+prune
// del tombstone leave, il nodo uscito non venga più targettato oltre la finestra attesa.
func TestVoluntaryLeaveNodeNotTargetAfterProtocolWindow(t *testing.T) {
	membershipCfg := membership.Config{
		SuspectTimeout: leaveSuspectTimeout,
		DeadTimeout:    leaveDeadTimeout,
		PruneRetention: leavePruneRetention,
	}

	network := newIntegrationNetwork()
	nodes, cancel := bootstrapClusterWithMembershipConfig(
		t,
		network,
		leaveAggregation,
		[]float64{10, 30, 50},
		leaveGossipInterval,
		membershipCfg,
	)
	defer cancel()
	defer stopCluster(t, nodes)

	leaving := nodes[0]
	if err := leaving.engine.AnnounceLeave(context.Background()); err != nil {
		t.Fatalf("annuncio leave fallito: %v", err)
	}
	if err := leaving.engine.Stop(); err != nil {
		t.Fatalf("stop nodo in leave fallito: %v", err)
	}
	nodes[0] = nil

	residualNodes := []*clusterNode{nodes[1], nodes[2]}
	_, pruned := waitForCondition(leavePruneDeadline, leavePollInterval, func() clusterObservation {
		return clusterObservation{}
	}, func(clusterObservation) bool {
		for _, observer := range residualNodes {
			if _, exists := snapshotMembershipPeer(observer, leaving.address); exists {
				return false
			}
		}
		return true
	})
	if !pruned {
		t.Fatalf("il nodo %s non è stato rimosso dalla membership entro %s", leaving.address, leavePruneDeadline)
	}

	before := network.deliveriesTo(leaving.address)
	time.Sleep(leaveIdleWindow)
	after := network.deliveriesTo(leaving.address)
	if after > before {
		t.Fatalf("il nodo leave %s è stato ancora targettato oltre la finestra di protocollo: before=%d after=%d", leaving.address, before, after)
	}
}
