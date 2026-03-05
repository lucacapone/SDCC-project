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
	now := time.Now().UTC()
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2", Status: membership.Suspect, Incarnation: 7, LastSeen: now})

	eng := NewEngine("node-1", "average", tr, m, nil, time.Second)
	eng.round(context.Background())

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
	now := time.Now().UTC()
	msgA := []shared.MembershipEntry{
		{NodeID: "node-b", Addr: "node-b", Status: string(membership.Suspect), Incarnation: 2, LastSeen: now.Add(1 * time.Second)},
		{NodeID: "node-c", Addr: "node-c", Status: string(membership.Alive), Incarnation: 1, LastSeen: now.Add(1 * time.Second)},
	}
	msgB := []shared.MembershipEntry{
		{NodeID: "node-b", Addr: "node-b", Status: string(membership.Alive), Incarnation: 3, LastSeen: now.Add(2 * time.Second)},
		{NodeID: "node-c", Addr: "node-c", Status: string(membership.Dead), Incarnation: 1, LastSeen: now.Add(2 * time.Second)},
	}
	msgC := []shared.MembershipEntry{
		{NodeID: "node-c", Addr: "node-c", Status: string(membership.Left), Incarnation: 4, LastSeen: now.Add(3 * time.Second)},
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
