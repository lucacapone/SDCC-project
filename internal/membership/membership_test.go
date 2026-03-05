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

func TestTouchResetsSuspected(t *testing.T) {
	set := NewSet()
	base := time.Now().UTC()
	set.Join("node-2:7002", base.Add(-10*time.Second))

	suspected := set.MarkSuspected(base, time.Second)
	if len(suspected) != 1 || !suspected[0].Suspected {
		t.Fatalf("peer non marcato come sospetto: %+v", suspected)
	}

	touchTime := base.Add(2 * time.Second)
	set.Touch("node-2:7002", touchTime)

	peers := set.Snapshot()
	if len(peers) != 1 {
		t.Fatalf("atteso un peer, got=%d", len(peers))
	}
	if peers[0].Suspected {
		t.Fatalf("touch avrebbe dovuto azzerare stato sospetto")
	}
	if !peers[0].LastSeen.Equal(touchTime) {
		t.Fatalf("last_seen inatteso: got=%v want=%v", peers[0].LastSeen, touchTime)
	}
}

func TestMarkSuspectedRespectsTimeout(t *testing.T) {
	set := NewSet()
	now := time.Now().UTC()
	set.Join("node-recent:7001", now.Add(-500*time.Millisecond))
	set.Join("node-old:7002", now.Add(-3*time.Second))

	suspected := set.MarkSuspected(now, time.Second)
	if len(suspected) != 1 {
		t.Fatalf("numero sospetti inatteso: got=%d want=1", len(suspected))
	}
	if suspected[0].Address != "node-old:7002" {
		t.Fatalf("peer sospetto inatteso: %+v", suspected[0])
	}
}
