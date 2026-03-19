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
	now := time.Now().UTC()
	join := &inMemoryJoinTransport{response: JoinResponse{
		Snapshot: []Peer{{NodeID: "node-2", Addr: "node2:7002", Status: Alive, LastSeen: now}},
		Delta:    []Peer{{NodeID: "node-3", Addr: "node3:7003", Status: Alive, LastSeen: now}},
	}}

	res := Bootstrap(context.Background(), set, JoinRequest{NodeID: "node-1", Addr: "node1:7001"}, "bootstrap:9000", []string{"seed-1:7001"}, join, now)

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
	now := time.Now().UTC()
	join := &inMemoryJoinTransport{err: errors.New("join down")}

	res := Bootstrap(context.Background(), set, JoinRequest{NodeID: "node-1", Addr: "node1:7001"}, "bootstrap:9000", []string{"node1:7001", "node2:7002", "node3:7003"}, join, now)

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
	now := time.Now().UTC()
	join := &inMemoryJoinTransport{response: JoinResponse{
		Snapshot: []Peer{
			{NodeID: "node-1", Addr: "node1:7001", Status: Alive, LastSeen: now},
			{NodeID: "seed-placeholder", Addr: "node1:7001", Status: Alive, LastSeen: now},
			{NodeID: "node-2", Addr: "node2:7002", Status: Alive, LastSeen: now},
		},
	}}

	res := Bootstrap(context.Background(), set, JoinRequest{NodeID: "node-1", Addr: "node1:7001"}, "bootstrap:9000", nil, join, now)

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
	now := time.Now().UTC()

	set.Join("node2:7002", now)
	set.Upsert(Peer{NodeID: "node-2", Addr: "node2:7002", Status: Alive, Incarnation: 1, LastSeen: now.Add(time.Second)})

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
