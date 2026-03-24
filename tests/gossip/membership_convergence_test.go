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

	eng := NewEngine("node-1", "average", tr, m, nil, nil, time.Second)
	eng.RoundOnce(context.Background())

	if len(tr.sent) == 0 {
		t.Fatalf("nessun messaggio inviato nel round gossip")
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

func TestRoundSerializzaMembershipEscludendoSelfNode(t *testing.T) {
	tr := &captureTransport{}
	m := membership.NewSet()
	base := time.Now().UTC()
	m.Upsert(membership.Peer{NodeID: "node-1", Addr: "node-1:7001", Status: membership.Alive, Incarnation: 10, LastSeen: base})
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, Incarnation: 3, LastSeen: base})

	eng := NewEngine("node-1", "average", tr, m, nil, nil, time.Second)
	eng.RoundOnce(context.Background())

	if len(tr.sent) == 0 {
		t.Fatalf("nessun messaggio inviato nel round gossip")
	}

	msg := decodeMessage(t, tr.sent[0])
	if len(msg.Membership) != 1 {
		t.Fatalf("digest membership deve contenere solo peer remoti: got=%d want=1", len(msg.Membership))
	}
	if msg.Membership[0].NodeID != "node-2" {
		t.Fatalf("digest membership include entry inattesa: %+v", msg.Membership[0])
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

func TestMergeMembershipIgnoraEntryDelNodoLocale(t *testing.T) {
	base := time.Date(2026, time.March, 24, 12, 30, 0, 0, time.UTC)
	set := membership.NewSet()
	set.Upsert(membership.Peer{
		NodeID:      "node-1",
		Addr:        "node-1:7001",
		Status:      membership.Alive,
		Incarnation: 5,
		LastSeen:    base,
	})

	mergeMembershipWithSelf(set, "node-1", []shared.MembershipEntry{
		{
			NodeID:      "node-1",
			Addr:        "node-1:7001",
			Status:      string(membership.Dead),
			Incarnation: 999,
			LastSeen:    base.Add(10 * time.Second),
		},
		{
			NodeID:      "node-2",
			Addr:        "node-2:7002",
			Status:      string(membership.Alive),
			Incarnation: 1,
			LastSeen:    base.Add(10 * time.Second),
		},
		{
			NodeID:      "node1:7001",
			Addr:        "node1:7001",
			Status:      string(membership.Suspect),
			Incarnation: 100,
			LastSeen:    base.Add(10 * time.Second),
		},
	}, "node1:7001")

	snapshot := membershipByNodeID(set.Snapshot())
	if snapshot["node-1"].Status != membership.Alive || snapshot["node-1"].Incarnation != 5 {
		t.Fatalf("entry self non deve essere applicata dal merge remoto: %+v", snapshot["node-1"])
	}
	if _, ok := snapshot["node-2"]; !ok {
		t.Fatalf("entry remota valida non applicata: %+v", snapshot)
	}
	if _, ok := snapshot["node1:7001"]; ok {
		t.Fatalf("alias self via addr non deve essere applicato: %+v", snapshot["node1:7001"])
	}
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

func TestMergeMembershipRecoversSuspectWithHigherAliveIncarnation(t *testing.T) {
	base := time.Date(2026, time.March, 23, 19, 30, 0, 0, time.UTC)
	set := membership.NewSetWithConfig(membership.Config{
		SuspectTimeout: time.Second,
		DeadTimeout:    2 * time.Second,
		PruneRetention: 10 * time.Second,
	})

	set.Upsert(membership.Peer{
		NodeID:      "node-b",
		Addr:        "node-b:7002",
		Status:      membership.Alive,
		Incarnation: 2,
		LastSeen:    base,
	})
	set.ApplyTimeoutTransitions(base.Add(1500 * time.Millisecond))

	peer, ok := membershipByNodeID(set.Snapshot())["node-b"]
	if !ok || peer.Status != membership.Suspect {
		t.Fatalf("precondizione non soddisfatta: peer suspect atteso, got=%+v", peer)
	}

	mergeMembership(set, []shared.MembershipEntry{{
		NodeID:      "node-b",
		Addr:        "node-b:7002",
		Status:      string(membership.Alive),
		Incarnation: 3,
		LastSeen:    base.Add(1600 * time.Millisecond),
	}})

	peer = membershipByNodeID(set.Snapshot())["node-b"]
	if peer.Status != membership.Alive {
		t.Fatalf("update alive con incarnation maggiore deve recuperare il peer: %+v", peer)
	}
	if peer.Incarnation != 3 {
		t.Fatalf("incarnation inattesa dopo il recupero: got=%d want=3", peer.Incarnation)
	}
}

func TestMergeMembershipIgnoresRejoinWithLowerIncarnation(t *testing.T) {
	base := time.Date(2026, time.March, 23, 19, 35, 0, 0, time.UTC)
	set := membership.NewSetWithConfig(membership.Config{
		SuspectTimeout: time.Second,
		DeadTimeout:    2 * time.Second,
		PruneRetention: 5 * time.Second,
	})

	set.Upsert(membership.Peer{
		NodeID:      "node-b",
		Addr:        "node-b:7002",
		Status:      membership.Left,
		Incarnation: 5,
		LastSeen:    base,
	})
	pruned := set.Prune(base.Add(5 * time.Second))
	if len(pruned) != 1 {
		t.Fatalf("prune inattesa: %+v", pruned)
	}

	mergeMembership(set, []shared.MembershipEntry{{
		NodeID:      "node-b",
		Addr:        "node-b:7002",
		Status:      string(membership.Alive),
		Incarnation: 4,
		LastSeen:    base.Add(6 * time.Second),
	}})

	if len(set.Snapshot()) != 0 {
		t.Fatalf("rejoin con incarnation obsoleta deve essere ignorato: %+v", set.Snapshot())
	}
}

func TestMergeMembershipRealignsPlaceholderSeedWithCanonicalNodeID(t *testing.T) {
	base := time.Date(2026, time.March, 23, 19, 40, 0, 0, time.UTC)
	set := membership.NewSet()

	// Il bootstrap seed-only crea un placeholder `host:port` usato come chiave provvisoria.
	set.Join("seed-a:7001", base)

	mergeMembership(set, []shared.MembershipEntry{{
		NodeID:      "node-a",
		Addr:        "seed-a:7001",
		Status:      string(membership.Alive),
		Incarnation: 2,
		LastSeen:    base.Add(time.Second),
	}})

	snapshot := membershipByNodeID(set.Snapshot())
	if _, exists := snapshot["seed-a:7001"]; exists {
		t.Fatalf("il placeholder seed host:port deve sparire dopo il riallineamento: %+v", set.Snapshot())
	}
	peer, ok := snapshot["node-a"]
	if !ok {
		t.Fatalf("node_id canonico non presente dopo il riallineamento: %+v", set.Snapshot())
	}
	if peer.Addr != "seed-a:7001" || peer.Status != membership.Alive || peer.Incarnation != 2 {
		t.Fatalf("peer riallineato inatteso: %+v", peer)
	}
}

func TestMergeMembershipReconvergesAfterTemporaryPartition(t *testing.T) {
	base := time.Date(2026, time.March, 23, 19, 55, 0, 0, time.UTC)

	leftNode := membership.NewSet()
	rightNode := membership.NewSet()

	// Durante la partizione i due sottoinsiemi sviluppano viste diverse della stessa membership.
	mergeMembership(leftNode, []shared.MembershipEntry{
		{NodeID: "node-2", Addr: "node-2:7002", Status: string(membership.Alive), Incarnation: 2, LastSeen: base.Add(1 * time.Second)},
		{NodeID: "node-3", Addr: "node-3:7003", Status: string(membership.Suspect), Incarnation: 3, LastSeen: base.Add(2 * time.Second)},
		{NodeID: "node-4", Addr: "node-4:7004", Status: string(membership.Suspect), Incarnation: 3, LastSeen: base.Add(2 * time.Second)},
	})
	mergeMembership(rightNode, []shared.MembershipEntry{
		{NodeID: "node-1", Addr: "node-1:7001", Status: string(membership.Suspect), Incarnation: 3, LastSeen: base.Add(2 * time.Second)},
		{NodeID: "node-2", Addr: "node-2:7002", Status: string(membership.Suspect), Incarnation: 3, LastSeen: base.Add(2 * time.Second)},
		{NodeID: "node-4", Addr: "node-4:7004", Status: string(membership.Alive), Incarnation: 2, LastSeen: base.Add(1 * time.Second)},
	})

	// Quando la partizione si chiude, ogni lato riceve update gossip `alive` con incarnation maggiore
	// provenienti dai peer tornati raggiungibili; duplicati e ordine invertito non devono impedire la riconvergenza.
	recoveryDigest := []shared.MembershipEntry{
		{NodeID: "node-1", Addr: "node-1:7001", Status: string(membership.Alive), Incarnation: 4, LastSeen: base.Add(3 * time.Second)},
		{NodeID: "node-2", Addr: "node-2:7002", Status: string(membership.Alive), Incarnation: 4, LastSeen: base.Add(3 * time.Second)},
		{NodeID: "node-3", Addr: "node-3:7003", Status: string(membership.Alive), Incarnation: 4, LastSeen: base.Add(3 * time.Second)},
		{NodeID: "node-4", Addr: "node-4:7004", Status: string(membership.Alive), Incarnation: 4, LastSeen: base.Add(3 * time.Second)},
	}

	mergeMembership(leftNode, recoveryDigest)
	mergeMembership(leftNode, recoveryDigest) // duplicato intenzionale
	mergeMembership(rightNode, recoveryDigest)
	mergeMembership(rightNode, []shared.MembershipEntry{
		{NodeID: "node-3", Addr: "node-3:7003", Status: string(membership.Suspect), Incarnation: 3, LastSeen: base.Add(2500 * time.Millisecond)},
	})

	expected := map[string]membership.Peer{
		"node-1": {NodeID: "node-1", Addr: "node-1:7001", Status: membership.Alive, Incarnation: 4},
		"node-2": {NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, Incarnation: 4},
		"node-3": {NodeID: "node-3", Addr: "node-3:7003", Status: membership.Alive, Incarnation: 4},
		"node-4": {NodeID: "node-4", Addr: "node-4:7004", Status: membership.Alive, Incarnation: 4},
	}
	assertMembership(t, leftNode.Snapshot(), expected)
	assertMembership(t, rightNode.Snapshot(), expected)
}

func TestMarkPeerAlivePromuoveAliasHostPortVersoNodeIDCanonico(t *testing.T) {
	base := time.Date(2026, time.March, 23, 23, 40, 0, 0, time.UTC)
	set := membership.NewSetWithConfig(membership.Config{
		SuspectTimeout: time.Second,
		DeadTimeout:    2 * time.Second,
		PruneRetention: 10 * time.Second,
	})

	set.Join("seed-a:7001", base)
	MarkPeerAliveForTest(set, "node-self", "node-a", "seed-a:7001", base.Add(500*time.Millisecond))

	snapshot := membershipByNodeID(set.Snapshot())
	if _, exists := snapshot["seed-a:7001"]; exists {
		t.Fatalf("l'alias host:port deve essere promosso al node_id canonico: %+v", set.Snapshot())
	}
	peer, ok := snapshot["node-a"]
	if !ok {
		t.Fatalf("peer canonico mancante dopo heartbeat implicito: %+v", set.Snapshot())
	}
	if peer.Addr != "seed-a:7001" || peer.Status != membership.Alive {
		t.Fatalf("peer canonicalizzato inatteso: %+v", peer)
	}

	set.ApplyTimeoutTransitions(base.Add(1300 * time.Millisecond))
	peer = membershipByNodeID(set.Snapshot())["node-a"]
	if peer.Status != membership.Alive {
		t.Fatalf("il peer canonicalizzato non deve ereditare transizioni suspect/dead del placeholder: %+v", peer)
	}
}

func TestSerializeMembershipDigestFiltraAliasObsoletoQuandoEsisteFormaCanonica(t *testing.T) {
	base := time.Date(2026, time.March, 23, 23, 45, 0, 0, time.UTC)
	entries := SerializeMembershipDigestForTest([]membership.Peer{
		{NodeID: "seed-a:7001", Addr: "seed-a:7001", Status: membership.Alive, LastSeen: base},
		{NodeID: "node-a", Addr: "seed-a:7001", Status: membership.Alive, Incarnation: 2, LastSeen: base.Add(time.Second)},
	})

	if len(entries) != 1 {
		t.Fatalf("digest inatteso, alias obsoleto non filtrato: %+v", entries)
	}
	if entries[0].NodeID != "node-a" || entries[0].Addr != "seed-a:7001" {
		t.Fatalf("entry canonica inattesa: %+v", entries[0])
	}
}

func TestSerializeMembershipDigestFiltraSempreSelfNode(t *testing.T) {
	base := time.Date(2026, time.March, 24, 13, 10, 0, 0, time.UTC)
	entries := SerializeMembershipDigestWithSelfForTest([]membership.Peer{
		{NodeID: "node-1", Addr: "node-1:7001", Status: membership.Alive, Incarnation: 3, LastSeen: base},
		{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Suspect, Incarnation: 7, LastSeen: base},
	}, "node-1")

	if len(entries) != 1 {
		t.Fatalf("digest deve escludere self node: %+v", entries)
	}
	if entries[0].NodeID != "node-2" {
		t.Fatalf("entry digest inattesa dopo filtro self: %+v", entries[0])
	}
}
