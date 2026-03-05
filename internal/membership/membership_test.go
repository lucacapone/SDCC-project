package membership

import (
	"testing"
	"time"
)

func TestJoinLeave(t *testing.T) {
	set := NewSet()
	now := time.Now().UTC()
	set.Join("node-2:7002", now)
	if got := len(set.Snapshot()); got != 1 {
		t.Fatalf("snapshot peers = %d, atteso 1", got)
	}
	set.Leave("node-2:7002")
	if got := len(set.Snapshot()); got != 0 {
		t.Fatalf("snapshot peers = %d, atteso 0", got)
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

func TestHigherIncarnationOverridesState(t *testing.T) {
	set := NewSet()
	base := time.Now().UTC()

	set.Upsert(Peer{NodeID: "node-2", Addr: "node-2:7002", Status: Dead, Incarnation: 1, LastSeen: base})
	set.Upsert(Peer{NodeID: "node-2", Addr: "node-2:7002", Status: Alive, Incarnation: 2, LastSeen: base.Add(2 * time.Second)})

	peer := byNodeID(set.Snapshot())["node-2"]
	if peer.Status != Alive {
		t.Fatalf("stato non ripristinato da incarnation maggiore: got=%s", peer.Status)
	}
	if peer.Incarnation != 2 {
		t.Fatalf("incarnation inattesa: got=%d want=2", peer.Incarnation)
	}
	if !peer.LastSeen.Equal(base.Add(2 * time.Second)) {
		t.Fatalf("last_seen inatteso: got=%v", peer.LastSeen)
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
