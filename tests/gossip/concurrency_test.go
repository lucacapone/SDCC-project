package gossip

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"sdcc-project/internal/membership"
	shared "sdcc-project/internal/types"
)

// TestRoundOnceConcurrentWithRemoteDelivery verifica che RoundOnce possa convivere
// con delivery remoto concorrente simulato dal transport spy.
func TestRoundOnceConcurrentWithRemoteDelivery(t *testing.T) {
	tr := &spyTransportEngine{}
	set := membership.NewSetWithConfig(membership.Config{
		SuspectTimeout: time.Second,
		DeadTimeout:    2 * time.Second,
		PruneRetention: 10 * time.Second,
	})
	base := time.Date(2026, time.April, 20, 9, 30, 0, 0, time.UTC)
	set.Upsert(membership.Peer{NodeID: "node-2", Addr: "node2:7002", Status: membership.Alive, Incarnation: 1, LastSeen: base})

	eng := NewEngine("node-1", "sum", tr, set, nil, nil, time.Hour, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	const rounds = 40
	const deliveries = 80

	start := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < rounds; i++ {
			eng.RoundOnce(context.Background())
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < deliveries; i++ {
			msg := simulatedRemoteMessage(i, base.Add(time.Duration(i)*time.Millisecond))
			raw, err := json.Marshal(msg)
			if err != nil {
				t.Errorf("marshal messaggio remoto: %v", err)
				return
			}
			if err := tr.deliver(context.Background(), raw); err != nil {
				t.Errorf("delivery remoto simulato: %v", err)
				return
			}
		}
	}()

	close(start)
	wg.Wait()

	if len(set.Snapshot()) == 0 {
		t.Fatal("membership vuota inattesa dopo round+delivery concorrenti")
	}
}

// TestConcurrentRoundOnceAndRemoteDeliveryInvariants valida invarianti di convergenza
// durante workload concorrente: no alias/canonico duplicati e incarnation non regressiva.
func TestConcurrentRoundOnceAndRemoteDeliveryInvariants(t *testing.T) {
	tr := &spyTransportEngine{}
	set := membership.NewSetWithConfig(membership.Config{
		SuspectTimeout: time.Second,
		DeadTimeout:    2 * time.Second,
		PruneRetention: 10 * time.Second,
	})
	base := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)

	set.Upsert(membership.Peer{NodeID: "node-2", Addr: "node2:7002", Status: membership.Alive, Incarnation: 1, LastSeen: base})
	set.Upsert(membership.Peer{NodeID: "node-3", Addr: "node3:7003", Status: membership.Alive, Incarnation: 1, LastSeen: base})

	eng := NewEngine("node-1", "sum", tr, set, nil, nil, time.Hour, 2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	const rounds = 120
	const deliveries = 150

	start := make(chan struct{})
	errCh := make(chan error, 1)

	var wg sync.WaitGroup

	// Auditor concorrente: legge snapshot mentre il carico è in corso e verifica
	// monotonicità dell'incarnation per node_id.
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		maxIncarnationByNode := map[string]uint64{}
		for i := 0; i < rounds+deliveries; i++ {
			snapshot := set.Snapshot()
			for _, peer := range snapshot {
				if prev, ok := maxIncarnationByNode[peer.NodeID]; ok && peer.Incarnation < prev {
					select {
					case errCh <- fmt.Errorf("incarnation regressiva: node=%s got=%d prev=%d", peer.NodeID, peer.Incarnation, prev):
					default:
					}
					return
				}
				if peer.Incarnation > maxIncarnationByNode[peer.NodeID] {
					maxIncarnationByNode[peer.NodeID] = peer.Incarnation
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < rounds; i++ {
			eng.RoundOnce(context.Background())
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < deliveries; i++ {
			msg := simulatedRemoteMessage(i, base.Add(time.Duration(i+1)*time.Millisecond))
			raw, err := json.Marshal(msg)
			if err != nil {
				select {
				case errCh <- fmt.Errorf("marshal messaggio remoto: %w", err):
				default:
				}
				return
			}
			if err := tr.deliver(context.Background(), raw); err != nil {
				select {
				case errCh <- fmt.Errorf("delivery remoto simulato: %w", err):
				default:
				}
				return
			}
		}
	}()

	close(start)
	wg.Wait()

	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}

	finalSnapshot := set.Snapshot()
	if len(finalSnapshot) == 0 {
		t.Fatal("snapshot finale vuoto inatteso")
	}

	// Invariante: non devono coesistere placeholder alias (node_id == addr)
	// e node_id canonico sullo stesso addr.
	canonicalByAddr := map[string]string{}
	aliasByAddr := map[string]bool{}
	for _, peer := range finalSnapshot {
		if peer.NodeID == peer.Addr {
			aliasByAddr[peer.Addr] = true
			continue
		}
		if existing, exists := canonicalByAddr[peer.Addr]; exists && existing != peer.NodeID {
			t.Fatalf("addr canonico duplicato: addr=%s first=%s second=%s", peer.Addr, existing, peer.NodeID)
		}
		canonicalByAddr[peer.Addr] = peer.NodeID
	}
	for addr := range aliasByAddr {
		if canonicalNode := canonicalByAddr[addr]; canonicalNode != "" {
			t.Fatalf("co-esistenza alias/canonico non ammessa: addr=%s canonical=%s", addr, canonicalNode)
		}
	}

	peerByNode := membershipByNodeID(finalSnapshot)
	if peerByNode["node-2"].Incarnation < 6 {
		t.Fatalf("incarnation finale node-2 troppo bassa: %+v", peerByNode["node-2"])
	}
	if peerByNode["node-3"].Incarnation < 6 {
		t.Fatalf("incarnation finale node-3 troppo bassa: %+v", peerByNode["node-3"])
	}
}

// simulatedRemoteMessage costruisce un messaggio remoto con digest che alterna
// entry canoniche e placeholder alias per stressare il merge membership concorrente.
func simulatedRemoteMessage(step int, sentAt time.Time) shared.GossipMessage {
	incarnation := uint64(step/25 + 2)
	status := membership.Alive
	if step%20 == 0 {
		status = membership.Suspect
	}

	membershipDigest := []shared.MembershipEntry{
		{
			NodeID:      "node-2",
			Addr:        "node2:7002",
			Status:      string(status),
			Incarnation: incarnation,
			LastSeen:    sentAt,
		},
		{
			NodeID:      "node-3",
			Addr:        "node3:7003",
			Status:      string(membership.Alive),
			Incarnation: incarnation,
			LastSeen:    sentAt,
		},
	}
	if step%3 == 0 {
		membershipDigest = append(membershipDigest, shared.MembershipEntry{
			NodeID:      "node2:7002",
			Addr:        "node2:7002",
			Status:      string(membership.Alive),
			Incarnation: incarnation,
			LastSeen:    sentAt,
		})
	}

	version := shared.StateVersion(step + 1)
	return shared.GossipMessage{
		MessageID:  shared.MessageID(fmt.Sprintf("remote-%03d", step)),
		OriginNode: "node-2",
		SentAt:     sentAt,
		Version:    currentMessageVersion,
		StateVersion: shared.StateVersionStamp{
			Epoch:   1,
			Counter: version,
		},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "sum",
			Value:           float64(10 + step),
			VersionEpoch:    1,
			VersionCounter:  version,
			Round:           version,
			UpdatedAt:       sentAt,
		},
		Membership: membershipDigest,
		Metadata: map[string]string{
			"origin_addr": "node2:7002",
		},
	}
}
