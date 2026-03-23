package max_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"testing"
	"time"

	"sdcc-project/internal/gossip"
	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
	shared "sdcc-project/internal/types"
)

// deterministicTransport è uno stub per consegne sincrone deterministiche.
type deterministicTransport struct {
	handler transport.MessageHandler
}

// Start registra l'handler sul transport fake.
func (d *deterministicTransport) Start(_ context.Context, h transport.MessageHandler) error {
	d.handler = h
	return nil
}

// Send non usa rete reale nei test.
func (d *deterministicTransport) Send(context.Context, string, []byte) error { return nil }

// Close non richiede teardown speciale nello stub.
func (d *deterministicTransport) Close() error { return nil }

// inject recapita subito il payload.
func (d *deterministicTransport) inject(ctx context.Context, payload []byte) error {
	if d.handler == nil {
		return fmt.Errorf("handler non inizializzato")
	}
	return d.handler(ctx, payload)
}

// testNode contiene engine e transport fake del nodo.
type testNode struct {
	eng *gossip.Engine
	tr  *deterministicTransport
}

// testHarness fornisce setup e consegna messaggi max.
type testHarness struct {
	nodes map[shared.NodeID]*testNode
}

// newTestHarness crea nodi max con ticker bloccato per round manuali.
func newTestHarness(t *testing.T, ids []shared.NodeID) *testHarness {
	t.Helper()
	h := &testHarness{nodes: make(map[shared.NodeID]*testNode, len(ids))}
	for _, id := range ids {
		tr := &deterministicTransport{}
		eng := gossip.NewEngine(string(id), "max", tr, membership.NewSet(), slog.Default(), nil, 24*time.Hour)
		eng.State.EnsureMergeMetadata()
		eng.State.EnsureMaxMetadata()

		ctx, cancel := context.WithCancel(context.Background())
		localEng := eng
		localCancel := cancel
		t.Cleanup(func() {
			localCancel()
			_ = localEng.Stop()
		})
		if err := eng.Start(ctx); err != nil {
			t.Fatalf("start engine %s: %v", id, err)
		}
		h.nodes[id] = &testNode{eng: eng, tr: tr}
	}
	return h
}

// setLocalValue inizializza valore/versione locale del nodo max.
func (h *testHarness) setLocalValue(id shared.NodeID, value float64) {
	n := h.nodes[id]
	n.eng.State.NodeID = id
	n.eng.State.AggregationType = "max"
	n.eng.State.Value = value
	n.eng.State.Round = 0
	n.eng.State.VersionCounter = 0
	n.eng.State.UpdatedAt = time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)
	n.eng.State.AggregationData.Max.Versions[id] = shared.StateVersionStamp{Counter: 0}
}

// deliverValue consegna update max dal nodo from al nodo to.
func (h *testHarness) deliverValue(t *testing.T, from, to shared.NodeID, messageID shared.MessageID, version shared.StateVersion, value float64, sentAt time.Time) {
	t.Helper()
	msg := shared.GossipMessage{
		MessageID:    messageID,
		OriginNode:   from,
		SentAt:       sentAt,
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: version},
		State: shared.GossipState{
			NodeID:          from,
			AggregationType: "max",
			Value:           value,
			Round:           version,
			VersionCounter:  version,
			UpdatedAt:       sentAt,
			AggregationData: shared.AggregationState{Max: &shared.MaxState{Versions: map[shared.NodeID]shared.StateVersionStamp{from: {Counter: version}}}},
		},
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal messaggio: %v", err)
	}
	if err := h.nodes[to].tr.inject(context.Background(), raw); err != nil {
		t.Fatalf("inject %s->%s: %v", from, to, err)
	}
}

// TestMaxConvergence copre convergenza, duplicati, out-of-order e nodo lento.
func TestMaxConvergence(t *testing.T) {
	base := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	t.Run("convergenza multi-nodo", func(t *testing.T) {
		ids := []shared.NodeID{"node-1", "node-2", "node-3", "node-4"}
		h := newTestHarness(t, ids)
		h.setLocalValue("node-1", 10)
		h.setLocalValue("node-2", 7)
		h.setLocalValue("node-3", 30)
		h.setLocalValue("node-4", 40)

		versionByReceiver := map[shared.NodeID]shared.StateVersion{}
		for _, to := range ids {
			for _, from := range ids {
				if from == to {
					continue
				}
				versionByReceiver[to] += 10
				h.deliverValue(t, from, to, shared.MessageID(fmt.Sprintf("%s-to-%s-v%d", from, to, versionByReceiver[to])), versionByReceiver[to], h.nodes[from].eng.State.Value, base)
			}
		}

		for _, id := range ids {
			if got := h.nodes[id].eng.State.Value; math.Abs(got-40) > 1e-9 {
				t.Fatalf("max non convergente su %s: got=%v want=40", id, got)
			}
		}
	})

	t.Run("duplicate update", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-2"})
		h.setLocalValue("node-1", 10)
		h.setLocalValue("node-2", 18)

		h.deliverValue(t, "node-2", "node-1", "dup", 1, 18, base)
		first := h.nodes["node-1"].eng.State.Value
		h.deliverValue(t, "node-2", "node-1", "dup", 1, 18, base)
		second := h.nodes["node-1"].eng.State.Value

		if math.Abs(first-second) > 1e-9 || math.Abs(second-18) > 1e-9 {
			t.Fatalf("duplicate non idempotente: first=%v second=%v", first, second)
		}
	})

	t.Run("out-of-order", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-2"})
		h.setLocalValue("node-1", 10)
		h.setLocalValue("node-2", 8)

		h.deliverValue(t, "node-2", "node-1", "v5", 5, 16, base.Add(time.Minute))
		afterNew := h.nodes["node-1"].eng.State.Value
		h.deliverValue(t, "node-2", "node-1", "v4-stale", 4, 9, base.Add(2*time.Minute))
		afterStale := h.nodes["node-1"].eng.State.Value

		if math.Abs(afterNew-16) > 1e-9 || math.Abs(afterStale-16) > 1e-9 {
			t.Fatalf("out-of-order non gestito: new=%v stale=%v", afterNew, afterStale)
		}
	})

	t.Run("nodo lento", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-2", "node-3"})
		h.setLocalValue("node-1", 10)
		h.setLocalValue("node-2", 20)
		h.setLocalValue("node-3", 35)

		h.deliverValue(t, "node-1", "node-2", "n1-v1", 1, 10, base)
		if got := h.nodes["node-2"].eng.State.Value; math.Abs(got-20) > 1e-9 {
			t.Fatalf("baseline inattesa: got=%v want=20", got)
		}
		h.deliverValue(t, "node-3", "node-2", "n3-v2-delayed", 2, 35, base.Add(250*time.Millisecond))
		if got := h.nodes["node-2"].eng.State.Value; math.Abs(got-35) > 1e-9 {
			t.Fatalf("update nodo lento non applicato: got=%v want=35", got)
		}
	})
}
