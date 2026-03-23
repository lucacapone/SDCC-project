package gossip

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sdcc-project/internal/membership"
	shared "sdcc-project/internal/types"
)

func TestRoundSerializzaMembershipConIncarnation(t *testing.T) {
	tr := &captureTransport{}
	m := membership.NewSet()
	base := time.Now().UTC()
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2", Status: membership.Suspect, Incarnation: 7, LastSeen: base})

	eng := NewEngine("node-1", "average", tr, m, nil, time.Second)
	eng.RoundOnce(context.Background())

	if len(tr.sent) != 1 {
		t.Fatalf("messaggi inviati inattesi: got=%d want=1", len(tr.sent))
	}

	msg := decodeMessage(t, tr.sent[0])
	if len(msg.Membership) != 1 {
		t.Fatalf("digest membership inatteso: got=%d want=1", len(msg.Membership))
	}
	entry := msg.Membership[0]
	if entry.NodeID != "node-2" || entry.Status != string(membership.Suspect) || entry.Incarnation != 7 {
		t.Fatalf("entry membership inattesa: %+v", entry)
	}
}

func TestMergeMembershipConvergeConDuplicatiOutOfOrder(t *testing.T) {
	base := time.Date(2026, time.March, 19, 11, 35, 0, 0, time.UTC)
	msgA := []shared.MembershipEntry{
		{NodeID: "node-b", Addr: "node-b", Status: string(membership.Suspect), Incarnation: 2, LastSeen: base.Add(1 * time.Second)},
		{NodeID: "node-c", Addr: "node-c", Status: string(membership.Alive), Incarnation: 1, LastSeen: base.Add(1 * time.Second)},
	}
	msgB := []shared.MembershipEntry{
		{NodeID: "node-b", Addr: "node-b", Status: string(membership.Alive), Incarnation: 3, LastSeen: base.Add(2 * time.Second)},
		{NodeID: "node-c", Addr: "node-c", Status: string(membership.Dead), Incarnation: 1, LastSeen: base.Add(2 * time.Second)},
	}
	msgC := []shared.MembershipEntry{
		{NodeID: "node-c", Addr: "node-c", Status: string(membership.Left), Incarnation: 4, LastSeen: base.Add(3 * time.Second)},
	}

	set1 := membership.NewSet()
	set2 := membership.NewSet()
	set3 := membership.NewSet()

	mergeMembership(set1, msgA)
	mergeMembership(set1, msgB)
	mergeMembership(set1, msgA) // duplicato
	mergeMembership(set1, msgC)

	mergeMembership(set2, msgC)
	mergeMembership(set2, msgA) // obsoleto su node-c
	mergeMembership(set2, msgB)

	mergeMembership(set3, msgB)
	mergeMembership(set3, msgA) // out-of-order
	mergeMembership(set3, msgC)
	mergeMembership(set3, msgB) // duplicato obsoleto

	expected := map[string]membership.Peer{
		"node-b": {NodeID: "node-b", Addr: "node-b", Status: membership.Alive, Incarnation: 3},
		"node-c": {NodeID: "node-c", Addr: "node-c", Status: membership.Left, Incarnation: 4},
	}
	assertMembership(t, set1.Snapshot(), expected)
	assertMembership(t, set2.Snapshot(), expected)
	assertMembership(t, set3.Snapshot(), expected)
}

func assertMembership(t *testing.T, got []membership.Peer, expected map[string]membership.Peer) {
	t.Helper()
	if len(got) != len(expected) {
		t.Fatalf("dimensione membership inattesa: got=%d want=%d", len(got), len(expected))
	}
	for _, peer := range got {
		exp, ok := expected[peer.NodeID]
		if !ok {
			t.Fatalf("peer inatteso: %+v", peer)
		}
		if peer.Status != exp.Status || peer.Incarnation != exp.Incarnation {
			t.Fatalf("peer %s inatteso: got=%+v want=%+v", peer.NodeID, peer, exp)
		}
	}
}

func decodeMessage(t *testing.T, payload []byte) shared.GossipMessage {
	t.Helper()
	var msg shared.GossipMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("decode messaggio: %v", err)
	}
	return msg
}

func TestMergeMembershipIgnoresObsoleteDigestAfterPrune(t *testing.T) {
	base := time.Date(2026, time.March, 23, 10, 30, 0, 0, time.UTC)
	set := membership.NewSetWithConfig(membership.Config{
		SuspectTimeout: time.Second,
		DeadTimeout:    2 * time.Second,
		PruneRetention: 5 * time.Second,
	})

	set.Upsert(membership.Peer{NodeID: "node-b", Addr: "node-b", Status: membership.Left, Incarnation: 4, LastSeen: base})
	pruned := set.Prune(base.Add(5 * time.Second))
	if len(pruned) != 1 {
		t.Fatalf("prune inattesa: %+v", pruned)
	}

	mergeMembership(set, []shared.MembershipEntry{{
		NodeID:      "node-b",
		Addr:        "node-b",
		Status:      string(membership.Alive),
		Incarnation: 4,
		LastSeen:    base.Add(6 * time.Second),
	}})
	if len(set.Snapshot()) != 0 {
		t.Fatalf("digest obsoleto non deve reintrodurre il peer: %+v", set.Snapshot())
	}

	mergeMembership(set, []shared.MembershipEntry{{
		NodeID:      "node-b",
		Addr:        "node-b",
		Status:      string(membership.Alive),
		Incarnation: 5,
		LastSeen:    base.Add(7 * time.Second),
	}})
	peer, ok := membershipByNodeID(set.Snapshot())["node-b"]
	if !ok {
		t.Fatalf("digest piu recente deve permettere il rejoin")
	}
	if peer.Status != membership.Alive || peer.Incarnation != 5 {
		t.Fatalf("peer inatteso dopo rejoin: %+v", peer)
	}
}

func membershipByNodeID(peers []membership.Peer) map[string]membership.Peer {
	out := make(map[string]membership.Peer, len(peers))
	for _, peer := range peers {
		out[peer.NodeID] = peer
	}
	return out
}
