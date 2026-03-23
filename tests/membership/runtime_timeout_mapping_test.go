package membership

import (
	"testing"
	"time"

	internalconfig "sdcc-project/internal/config"
)

// TestMembershipTimeoutRuntimeMappingRendeSuspectEDeadOsservabili verifica che
// il valore runtime membership_timeout_ms cambi davvero le transizioni
// osservabili della membership quando viene tradotto nella Config interna.
func TestMembershipTimeoutRuntimeMappingRendeSuspectEDeadOsservabili(t *testing.T) {
	base := time.Date(2026, time.March, 23, 12, 0, 0, 0, time.UTC)

	shortRuntimeCfg := internalconfig.Config{MembershipTimeoutMS: 100}
	longRuntimeCfg := internalconfig.Config{MembershipTimeoutMS: 400}

	shortSet := NewSetWithConfig(shortRuntimeCfg.MembershipConfig())
	longSet := NewSetWithConfig(longRuntimeCfg.MembershipConfig())

	shortSet.Upsert(Peer{NodeID: "node-short", Addr: "node-short:7001", Status: Alive, LastSeen: base})
	longSet.Upsert(Peer{NodeID: "node-long", Addr: "node-long:7002", Status: Alive, LastSeen: base})

	// Dopo 75ms il timeout corto deve già entrare in suspect, mentre quello lungo
	// deve restare alive perché la sua soglia suspect è 200ms.
	shortSet.ApplyTimeoutTransitions(base.Add(75 * time.Millisecond))
	longSet.ApplyTimeoutTransitions(base.Add(75 * time.Millisecond))

	shortPeer := byNodeID(shortSet.Snapshot())["node-short"]
	longPeer := byNodeID(longSet.Snapshot())["node-long"]
	if shortPeer.Status != Suspect {
		t.Fatalf("timeout corto: atteso suspect dopo 75ms, got=%s", shortPeer.Status)
	}
	if longPeer.Status != Alive {
		t.Fatalf("timeout lungo: atteso alive dopo 75ms, got=%s", longPeer.Status)
	}

	// Dopo 150ms il timeout corto deve risultare dead, mentre quello lungo deve
	// essere solo suspect perché la soglia dead è 400ms.
	shortSet.ApplyTimeoutTransitions(base.Add(150 * time.Millisecond))
	longSet.ApplyTimeoutTransitions(base.Add(150 * time.Millisecond))

	shortPeer = byNodeID(shortSet.Snapshot())["node-short"]
	longPeer = byNodeID(longSet.Snapshot())["node-long"]
	if shortPeer.Status != Dead {
		t.Fatalf("timeout corto: atteso dead dopo 150ms, got=%s", shortPeer.Status)
	}
	if longPeer.Status != Alive {
		t.Fatalf("timeout lungo: atteso ancora alive dopo 150ms, got=%s", longPeer.Status)
	}

	// Dopo 250ms il timeout lungo supera finalmente la soglia suspect e rende la
	// differenza osservabile fra le due configurazioni runtime.
	longSet.ApplyTimeoutTransitions(base.Add(250 * time.Millisecond))
	longPeer = byNodeID(longSet.Snapshot())["node-long"]
	if longPeer.Status != Suspect {
		t.Fatalf("timeout lungo: atteso suspect dopo 250ms, got=%s", longPeer.Status)
	}
}

// TestMembershipTimeoutRuntimeMappingGarantisceFinestraSuspectAncheConValoriMinimi
// congela la regola di normalizzazione minima, così anche 1ms produce una
// finestra suspect osservabile invece di saltare direttamente a dead.
func TestMembershipTimeoutRuntimeMappingGarantisceFinestraSuspectAncheConValoriMinimi(t *testing.T) {
	runtimeCfg := internalconfig.Config{MembershipTimeoutMS: 1}
	membershipCfg := runtimeCfg.MembershipConfig()

	if membershipCfg.SuspectTimeout != time.Millisecond {
		t.Fatalf("suspect timeout minimo inatteso: got=%s want=1ms", membershipCfg.SuspectTimeout)
	}
	if membershipCfg.DeadTimeout != 2*time.Millisecond {
		t.Fatalf("dead timeout minimo inatteso: got=%s want=2ms", membershipCfg.DeadTimeout)
	}

	set := NewSetWithConfig(membershipCfg)
	base := time.Date(2026, time.March, 23, 12, 5, 0, 0, time.UTC)
	set.Upsert(Peer{NodeID: "node-min", Addr: "node-min:7001", Status: Alive, LastSeen: base})

	set.ApplyTimeoutTransitions(base.Add(1500 * time.Microsecond))
	peer := byNodeID(set.Snapshot())["node-min"]
	if peer.Status != Suspect {
		t.Fatalf("atteso suspect nella finestra minima osservabile, got=%s", peer.Status)
	}

	set.ApplyTimeoutTransitions(base.Add(3 * time.Millisecond))
	peer = byNodeID(set.Snapshot())["node-min"]
	if peer.Status != Dead {
		t.Fatalf("atteso dead oltre la soglia minima, got=%s", peer.Status)
	}
}
