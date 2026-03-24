package gossip

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
	shared "sdcc-project/internal/types"
)

// spyTransportEngine verifica che Engine usi esclusivamente l'interfaccia Transport.
type spyTransportEngine struct {
	mu          sync.Mutex
	startCalled bool
	closeCalled bool
	sendCalls   int
	handler     transport.MessageHandler
}

func (s *spyTransportEngine) Start(_ context.Context, handler transport.MessageHandler) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalled = true
	s.handler = handler
	return nil
}

func (s *spyTransportEngine) Send(_ context.Context, _ string, _ []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendCalls++
	return nil
}

func (s *spyTransportEngine) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCalled = true
	return nil
}

func (s *spyTransportEngine) deliver(ctx context.Context, payload []byte) error {
	s.mu.Lock()
	h := s.handler
	s.mu.Unlock()
	if h == nil {
		return errors.New("handler non registrato")
	}
	return h(ctx, payload)
}

func TestEngineUsaSoloInterfacciaTransportStartStop(t *testing.T) {
	tr := &spyTransportEngine{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng := NewEngine("node-1", "sum", tr, membership.NewSet(), logger, nil, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	if err := eng.Stop(); err != nil {
		t.Fatalf("stop engine errore: %v", err)
	}

	tr.mu.Lock()
	defer tr.mu.Unlock()
	if !tr.startCalled {
		t.Fatal("engine non ha invocato Transport.Start")
	}
	if !tr.closeCalled {
		t.Fatal("engine non ha invocato Transport.Close")
	}
}

func TestEngineGestisceMessaggiInIngressoViaHandlerTransport(t *testing.T) {
	tr := &spyTransportEngine{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mset := membership.NewSet()
	eng := NewEngine("node-1", "sum", tr, mset, logger, nil, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	now := time.Unix(1710000000, 0).UTC()
	incoming := shared.GossipMessage{
		MessageID:  "m-1",
		OriginNode: "node-2",
		SentAt:     now,
		Version:    currentMessageVersion,
		StateVersion: shared.StateVersionStamp{
			Epoch:   1,
			Counter: 1,
		},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "sum",
			VersionEpoch:    1,
			VersionCounter:  1,
			Round:           1,
			UpdatedAt:       now,
		},
		Membership: []shared.MembershipEntry{{
			NodeID:      "node-2",
			Addr:        "node-2:7002",
			Status:      string(membership.Alive),
			Incarnation: 2,
			LastSeen:    now,
		}},
	}
	raw, err := json.Marshal(incoming)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := tr.deliver(context.Background(), raw); err != nil {
		t.Fatalf("deliver handler: %v", err)
	}

	lastSender := eng.State.LastSenderNodeID
	versionCounter := eng.State.VersionCounter
	if lastSender != "node-2" {
		t.Fatalf("last sender inatteso: got=%q want=node-2", lastSender)
	}
	if versionCounter != 2 {
		t.Fatalf("version counter inatteso: got=%d want=2", versionCounter)
	}

	var peer membership.Peer
	found := false
	for _, candidate := range mset.Snapshot() {
		if candidate.NodeID == "node-2" {
			peer = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatal("membership non aggiornata da messaggio ricevuto")
	}
	if peer.Addr != "node-2:7002" {
		t.Fatalf("addr peer inatteso: got=%q want=node-2:7002", peer.Addr)
	}
}

func TestEngineHeartbeatImplicitoSenzaSelfNelDigestMantieneEndpointCanonico(t *testing.T) {
	tr := &spyTransportEngine{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := time.Date(2026, time.March, 24, 10, 0, 0, 0, time.UTC)
	mset := membership.NewSetWithConfig(membership.Config{
		SuspectTimeout: time.Second,
		DeadTimeout:    2 * time.Second,
		PruneRetention: 10 * time.Second,
	})
	mset.Upsert(membership.Peer{
		NodeID:      "node-2",
		Addr:        "node-2:7002",
		Status:      membership.Alive,
		Incarnation: 4,
		LastSeen:    base,
	})
	eng := NewEngine("node-1", "sum", tr, mset, logger, nil, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	heartbeatAt := base.Add(900 * time.Millisecond)
	incoming := shared.GossipMessage{
		MessageID:  "m-missing-self-entry",
		OriginNode: "node-2",
		SentAt:     heartbeatAt,
		Version:    currentMessageVersion,
		StateVersion: shared.StateVersionStamp{
			Epoch:   1,
			Counter: 5,
		},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "sum",
			Value:           21,
			VersionEpoch:    1,
			VersionCounter:  5,
			Round:           9,
			UpdatedAt:       heartbeatAt,
		},
		Membership: []shared.MembershipEntry{{
			NodeID:      "node-3",
			Addr:        "node-3:7003",
			Status:      string(membership.Alive),
			Incarnation: 2,
			LastSeen:    heartbeatAt,
		}},
	}
	raw, err := json.Marshal(incoming)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	deliverCtx := transport.WithMessageRemoteAddr(context.Background(), "node-2:7002")
	if err := tr.deliver(deliverCtx, raw); err != nil {
		t.Fatalf("deliver handler: %v", err)
	}

	peer, ok := membershipByNodeID(mset.Snapshot())["node-2"]
	if !ok {
		t.Fatalf("peer origin mancante dopo heartbeat implicito: %+v", mset.Snapshot())
	}
	if peer.Addr != "node-2:7002" {
		t.Fatalf("addr del peer origin alterato: got=%q want=node-2:7002", peer.Addr)
	}
	if peer.Status != membership.Alive {
		t.Fatalf("stato peer inatteso dopo heartbeat implicito: got=%s want=alive", peer.Status)
	}

	mset.ApplyTimeoutTransitions(heartbeatAt.Add(500 * time.Millisecond))
	peer = membershipByNodeID(mset.Snapshot())["node-2"]
	if peer.Status != membership.Alive {
		t.Fatalf("cluster sano: il peer non deve degradare dopo heartbeat implicito: %+v", peer)
	}
}
