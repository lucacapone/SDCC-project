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
	lastReq  JoinRequest
}

func (t *inMemoryJoinTransport) Join(_ context.Context, _ string, req JoinRequest) (JoinResponse, error) {
	t.calls++
	t.lastReq = req
	if t.err != nil {
		return JoinResponse{}, t.err
	}
	return t.response, nil
}

func TestBootstrapJoinDiscoveryAppliesInitialView(t *testing.T) {
	set := NewSet()
	base := time.Date(2026, time.March, 19, 11, 0, 0, 0, time.UTC)
	join := &inMemoryJoinTransport{response: JoinResponse{
		Snapshot: []Peer{{NodeID: "node-2", Addr: "node2:7002", Status: Alive, LastSeen: base.Add(1 * time.Second)}},
		Delta:    []Peer{{NodeID: "node-3", Addr: "node3:7003", Status: Alive, LastSeen: base.Add(2 * time.Second)}},
	}}

	res := Bootstrap(context.Background(), set, JoinRequest{NodeID: "node-1", Addr: "node1:7001"}, "bootstrap:9000", []string{"seed-1:7001"}, join, base)

	if !res.UsedJoinEndpoint || res.FallbackUsed {
		t.Fatalf("bootstrap result inatteso: %+v", res)
	}
	if join.calls != 1 {
		t.Fatalf("join non invocato correttamente: %d", join.calls)
	}
	if join.lastReq.NodeID != "node-1" || join.lastReq.Addr != "node1:7001" {
		t.Fatalf("join request inattesa: %+v", join.lastReq)
	}
	if got := len(set.Snapshot()); got != 2 {
		t.Fatalf("membership iniziale inattesa: %d", got)
	}
}

func TestBootstrapFallbackToStaticSeedsWhenJoinUnavailable(t *testing.T) {
	set := NewSet()
	base := time.Date(2026, time.March, 19, 11, 5, 0, 0, time.UTC)
	join := &inMemoryJoinTransport{err: errors.New("join down")}

	res := Bootstrap(context.Background(), set, JoinRequest{NodeID: "node-1", Addr: "node1:7001"}, "bootstrap:9000", []string{"node1:7001", "node2:7002", "node3:7003"}, join, base)

	if res.UsedJoinEndpoint || !res.FallbackUsed {
		t.Fatalf("bootstrap result inatteso: %+v", res)
	}
	peers := byNodeID(set.Snapshot())
	if got := len(peers); got != 2 {
		t.Fatalf("fallback seed peers non applicato correttamente: %d", got)
	}
	if _, ok := peers["node2:7002"]; !ok {
		t.Fatalf("seed node2 mancante: %+v", peers)
	}
	if _, ok := peers["node3:7003"]; !ok {
		t.Fatalf("seed node3 mancante: %+v", peers)
	}
}

func TestBootstrapJoinDiscoverySkipsSelfByLogicalIDAndEndpoint(t *testing.T) {
	set := NewSet()
	base := time.Date(2026, time.March, 19, 11, 10, 0, 0, time.UTC)
	join := &inMemoryJoinTransport{response: JoinResponse{
		Snapshot: []Peer{
			{NodeID: "node-1", Addr: "node1:7001", Status: Alive, LastSeen: base.Add(1 * time.Second)},
			{NodeID: "seed-placeholder", Addr: "node1:7001", Status: Alive, LastSeen: base.Add(2 * time.Second)},
			{NodeID: "node-2", Addr: "node2:7002", Status: Alive, LastSeen: base.Add(3 * time.Second)},
		},
	}}

	res := Bootstrap(context.Background(), set, JoinRequest{NodeID: "node-1", Addr: "node1:7001"}, "bootstrap:9000", nil, join, base)

	if !res.UsedJoinEndpoint {
		t.Fatalf("join endpoint non usato: %+v", res)
	}
	peers := byNodeID(set.Snapshot())
	if len(peers) != 1 {
		t.Fatalf("self peer non filtrato correttamente: %+v", peers)
	}
	if _, ok := peers["node-2"]; !ok {
		t.Fatalf("peer remoto mancante: %+v", peers)
	}
}

func TestUpsertPromotesSeedPlaceholderToLogicalNodeID(t *testing.T) {
	set := NewSet()
	base := time.Date(2026, time.March, 19, 11, 15, 0, 0, time.UTC)

	set.Join("node2:7002", base)
	set.Upsert(Peer{NodeID: "node-2", Addr: "node2:7002", Status: Alive, Incarnation: 1, LastSeen: base.Add(1 * time.Second)})

	peers := byNodeID(set.Snapshot())
	if len(peers) != 1 {
		t.Fatalf("atteso un solo peer dopo riallineamento placeholder: %+v", peers)
	}
	if _, ok := peers["node2:7002"]; ok {
		t.Fatalf("placeholder seed non rimosso: %+v", peers)
	}
	peer, ok := peers["node-2"]
	if !ok {
		t.Fatalf("peer logico mancante dopo riallineamento: %+v", peers)
	}
	if peer.Addr != "node2:7002" {
		t.Fatalf("addr inatteso dopo riallineamento: %+v", peer)
	}
}
