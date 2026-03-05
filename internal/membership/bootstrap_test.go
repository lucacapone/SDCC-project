package membership

import (
	"context"
	"errors"
	"testing"
	"time"
)

type inMemoryJoinTransport struct {
	response JoinResponse
	err      error
	calls    int
}

func (t *inMemoryJoinTransport) Join(context.Context, string, JoinRequest) (JoinResponse, error) {
	t.calls++
	if t.err != nil {
		return JoinResponse{}, t.err
	}
	return t.response, nil
}

func TestBootstrapJoinDiscoveryAppliesInitialView(t *testing.T) {
	set := NewSet()
	now := time.Now().UTC()
	join := &inMemoryJoinTransport{response: JoinResponse{
		Snapshot: []Peer{{NodeID: "node-2", Addr: "node-2:7002", Status: Alive, LastSeen: now}},
		Delta:    []Peer{{NodeID: "node-3", Addr: "node-3:7003", Status: Alive, LastSeen: now}},
	}}

	res := Bootstrap(context.Background(), set, JoinRequest{NodeID: "node-1", Addr: "node-1:7001"}, "bootstrap:9000", []string{"seed-1"}, join, now)

	if !res.UsedJoinEndpoint || res.FallbackUsed {
		t.Fatalf("bootstrap result inatteso: %+v", res)
	}
	if join.calls != 1 {
		t.Fatalf("join non invocato correttamente: %d", join.calls)
	}
	if got := len(set.Snapshot()); got != 2 {
		t.Fatalf("membership iniziale inattesa: %d", got)
	}
}

func TestBootstrapFallbackToStaticSeedsWhenJoinUnavailable(t *testing.T) {
	set := NewSet()
	now := time.Now().UTC()
	join := &inMemoryJoinTransport{err: errors.New("join down")}

	res := Bootstrap(context.Background(), set, JoinRequest{NodeID: "node-1", Addr: "node-1:7001"}, "bootstrap:9000", []string{"seed-1:7001", "seed-2:7002"}, join, now)

	if res.UsedJoinEndpoint || !res.FallbackUsed {
		t.Fatalf("bootstrap result inatteso: %+v", res)
	}
	if got := len(set.Snapshot()); got != 2 {
		t.Fatalf("fallback seed peers non applicato: %d", got)
	}
}
