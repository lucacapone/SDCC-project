package membership

import (
	"context"
	"testing"
	"time"
)

func TestJoinLeave(t *testing.T) {
	cfg := Config{SuspectTimeout: 2 * time.Second, DeadTimeout: 4 * time.Second}
	setA := NewSetWithConfig(cfg)
	base := time.Date(2026, time.March, 5, 18, 30, 0, 0, time.UTC)

	join := &inMemoryJoinTransport{response: JoinResponse{
		Snapshot: []Peer{{
			NodeID:      "node-b",
			Addr:        "node-b:7002",
			Status:      Alive,
			Incarnation: 1,
			LastSeen:    base,
		}},
	}}

	res := Bootstrap(
		context.Background(),
		setA,
		JoinRequest{NodeID: "node-a", Addr: "node-a:7001"},
		"bootstrap:9000",
		nil,
		join,
		base,
	)
	if !res.UsedJoinEndpoint {
		t.Fatalf("join endpoint non usato: %+v", res)
	}

	peers := byNodeID(setA.Snapshot())
	if peers["node-b"].Status != Alive {
		t.Fatalf("node-b deve risultare alive dopo bootstrap: got=%s", peers["node-b"].Status)
	}

	setA.ApplyTimeoutTransitions(base.Add(3 * time.Second))
	peers = byNodeID(setA.Snapshot())
	if peers["node-b"].Status != Suspect {
		t.Fatalf("node-b deve passare a suspect dopo inattivita': got=%s", peers["node-b"].Status)
	}

	setA.ApplyTimeoutTransitions(base.Add(5 * time.Second))
	peers = byNodeID(setA.Snapshot())
	if peers["node-b"].Status != Dead {
		t.Fatalf("node-b deve passare a dead dopo timeout: got=%s", peers["node-b"].Status)
	}

	setA.Leave("node-b")
	peers = byNodeID(setA.Snapshot())
	if peers["node-b"].Status != Left {
		t.Fatalf("cleanup tombstone non applicato: got=%s", peers["node-b"].Status)
	}

	setA.ApplyTimeoutTransitions(base.Add(20 * time.Second))
	peers = byNodeID(setA.Snapshot())
	if peers["node-b"].Status != Left {
		t.Fatalf("tombstone leave non deve degradare: got=%s", peers["node-b"].Status)
	}
}

func TestTimeoutTransitions(t *testing.T) {
	cfg := Config{SuspectTimeout: 2 * time.Second, DeadTimeout: 5 * time.Second}
	set := NewSetWithConfig(cfg)
	base := time.Now().UTC()

	set.Upsert(Peer{NodeID: "alive", Addr: "alive:7001", Status: Alive, LastSeen: base.Add(-1 * time.Second)})
	set.Upsert(Peer{NodeID: "suspect", Addr: "suspect:7002", Status: Alive, LastSeen: base.Add(-3 * time.Second)})
	set.Upsert(Peer{NodeID: "dead", Addr: "dead:7003", Status: Alive, LastSeen: base.Add(-7 * time.Second)})

	set.ApplyTimeoutTransitions(base)
	peers := byNodeID(set.Snapshot())

	if peers["alive"].Status != Alive {
		t.Fatalf("alive inatteso: got=%s", peers["alive"].Status)
	}
	if peers["suspect"].Status != Suspect {
		t.Fatalf("suspect inatteso: got=%s", peers["suspect"].Status)
	}
	if peers["dead"].Status != Dead {
		t.Fatalf("dead inatteso: got=%s", peers["dead"].Status)
	}
}

func TestRejoinWithHigherIncarnationOverridesOldState(t *testing.T) {
	set := NewSetWithConfig(Config{SuspectTimeout: time.Second, DeadTimeout: 2 * time.Second})
	base := time.Date(2026, time.March, 5, 18, 40, 0, 0, time.UTC)

	set.Upsert(Peer{NodeID: "node-b", Addr: "node-b:7002", Status: Dead, Incarnation: 3, LastSeen: base})
	set.Upsert(Peer{NodeID: "node-b", Addr: "node-b:7002", Status: Alive, Incarnation: 4, LastSeen: base.Add(1500 * time.Millisecond)})

	peer := byNodeID(set.Snapshot())["node-b"]
	if peer.Status != Alive {
		t.Fatalf("rejoin con incarnation maggiore deve ripristinare alive: got=%s", peer.Status)
	}
	if peer.Incarnation != 4 {
		t.Fatalf("incarnation inattesa dopo rejoin: got=%d want=4", peer.Incarnation)
	}
	if !peer.LastSeen.Equal(base.Add(1500 * time.Millisecond)) {
		t.Fatalf("last_seen inatteso dopo rejoin: got=%v", peer.LastSeen)
	}
}

func TestGossipUpdateMitigatesFalsePositiveToAlive(t *testing.T) {
	cfg := Config{SuspectTimeout: 2 * time.Second, DeadTimeout: 4 * time.Second}
	set := NewSetWithConfig(cfg)
	base := time.Date(2026, time.March, 5, 18, 45, 0, 0, time.UTC)

	set.Upsert(Peer{NodeID: "node-b", Addr: "node-b:7002", Status: Alive, Incarnation: 1, LastSeen: base})
	set.ApplyTimeoutTransitions(base.Add(3 * time.Second))

	peer := byNodeID(set.Snapshot())["node-b"]
	if peer.Status != Suspect {
		t.Fatalf("precondizione non soddisfatta: node-b deve essere suspect, got=%s", peer.Status)
	}

	set.Upsert(Peer{NodeID: "node-b", Addr: "node-b:7002", Status: Alive, Incarnation: 2, LastSeen: base.Add(3500 * time.Millisecond)})
	peer = byNodeID(set.Snapshot())["node-b"]
	if peer.Status != Alive {
		t.Fatalf("update gossip alive deve mitigare falso positivo: got=%s", peer.Status)
	}

	set.ApplyTimeoutTransitions(base.Add(3800 * time.Millisecond))
	peer = byNodeID(set.Snapshot())["node-b"]
	if peer.Status != Alive {
		t.Fatalf("dopo update gossip recente node-b deve restare alive: got=%s", peer.Status)
	}
}

func TestLowerIncarnationIsIgnored(t *testing.T) {
	set := NewSet()
	base := time.Now().UTC()

	set.Upsert(Peer{NodeID: "node-2", Addr: "node-2:7002", Status: Suspect, Incarnation: 3, LastSeen: base})
	set.Upsert(Peer{NodeID: "node-2", Addr: "node-2:7002", Status: Alive, Incarnation: 2, LastSeen: base.Add(5 * time.Second)})

	peer := byNodeID(set.Snapshot())["node-2"]
	if peer.Incarnation != 3 {
		t.Fatalf("incarnation inattesa: got=%d want=3", peer.Incarnation)
	}
	if peer.Status != Suspect {
		t.Fatalf("stato inatteso: got=%s want=%s", peer.Status, Suspect)
	}
}

func byNodeID(peers []Peer) map[string]Peer {
	out := make(map[string]Peer, len(peers))
	for _, p := range peers {
		out[p.NodeID] = p
	}
	return out
}
