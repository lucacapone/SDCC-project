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
