package sum_test

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

// deterministicTransport è uno stub transport che permette consegne dirette e sincrone.
type deterministicTransport struct {
	handler transport.MessageHandler
}

// Start salva l'handler del nodo destinatario.
func (d *deterministicTransport) Start(_ context.Context, h transport.MessageHandler) error {
	d.handler = h
	return nil
}

// Send non usa rete reale in questi test.
func (d *deterministicTransport) Send(context.Context, string, []byte) error { return nil }

// Close chiude lo stub senza effetti collaterali.
func (d *deterministicTransport) Close() error { return nil }

// inject recapita immediatamente il payload al nodo.
func (d *deterministicTransport) inject(ctx context.Context, payload []byte) error {
	if d.handler == nil {
		return fmt.Errorf("handler non inizializzato")
	}
	return d.handler(ctx, payload)
}

// testNode incapsula engine e transport fake.
type testNode struct {
	eng *gossip.Engine
	tr  *deterministicTransport
}

// testHarness offre API deterministiche per setup e consegne.
type testHarness struct {
	nodes map[shared.NodeID]*testNode
}

// newTestHarness crea nodi sum con ticker lungo per evitare round automatici non deterministici.
func newTestHarness(t *testing.T, ids []shared.NodeID) *testHarness {
	t.Helper()

	h := &testHarness{nodes: make(map[shared.NodeID]*testNode, len(ids))}
	for _, id := range ids {
		tr := &deterministicTransport{}
		eng := gossip.NewEngine(string(id), "sum", tr, membership.NewSet(), slog.Default(), nil, 24*time.Hour)
		eng.State.EnsureMergeMetadata()
		eng.State.EnsureSumMetadata()

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

// setLocalContribution inizializza il contributo locale a versione zero.
func (h *testHarness) setLocalContribution(id shared.NodeID, value float64) {
	n := h.nodes[id]
	n.eng.State.NodeID = id
	n.eng.State.AggregationType = "sum"
	n.eng.State.Value = value
	n.eng.State.Round = 0
	n.eng.State.VersionCounter = 0
	n.eng.State.UpdatedAt = time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)
	n.eng.State.AggregationData.Sum.Contributions[id] = value
	n.eng.State.AggregationData.Sum.Versions[id] = shared.StateVersionStamp{Counter: 0}
}

// deliverSingleContribution consegna un messaggio che trasporta un solo contributo per nodo sorgente.
func (h *testHarness) deliverSingleContribution(t *testing.T, from, to shared.NodeID, messageID shared.MessageID, version shared.StateVersion, contribution float64, sentAt time.Time) {
	t.Helper()

	msg := shared.GossipMessage{
		MessageID:    messageID,
		OriginNode:   from,
		SentAt:       sentAt,
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: version},
		State: shared.GossipState{
			NodeID:          from,
			AggregationType: "sum",
			Value:           contribution,
			Round:           version,
			VersionCounter:  version,
			UpdatedAt:       sentAt,
			AggregationData: shared.AggregationState{Sum: &shared.SumState{
				Contributions: map[shared.NodeID]float64{from: contribution},
				Versions:      map[shared.NodeID]shared.StateVersionStamp{from: {Counter: version}},
			}},
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

// assertNodeValue verifica il valore numerico aggregato finale.
func (h *testHarness) assertNodeValue(t *testing.T, id shared.NodeID, want float64) {
	t.Helper()
	got := h.nodes[id].eng.State.Value
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("valore inatteso su %s: got=%v want=%v", id, got, want)
	}
}

// TestSumConvergence verifica convergenza e robustezza del merge sum con harness deterministico.
func TestSumConvergence(t *testing.T) {
	base := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	t.Run("convergenza su N nodi", func(t *testing.T) {
		ids := []shared.NodeID{"node-1", "node-2", "node-3", "node-4"}
		h := newTestHarness(t, ids)
		h.setLocalContribution("node-1", 10)
		h.setLocalContribution("node-2", 20)
		h.setLocalContribution("node-3", 30)
		h.setLocalContribution("node-4", 40)

		versionByReceiver := map[shared.NodeID]shared.StateVersion{}
		for _, to := range ids {
			for _, from := range ids {
				if from == to {
					continue
				}
				versionByReceiver[to] += 10
				contribution := h.nodes[from].eng.State.AggregationData.Sum.Contributions[from]
				h.deliverSingleContribution(t, from, to, shared.MessageID(fmt.Sprintf("%s-to-%s-v%d", from, to, versionByReceiver[to])), versionByReceiver[to], contribution, base)
			}
		}

		for _, id := range ids {
			h.assertNodeValue(t, id, 100)
		}
	})

	t.Run("duplicate update", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-2"})
		h.setLocalContribution("node-1", 10)
		h.setLocalContribution("node-2", 20)

		h.deliverSingleContribution(t, "node-2", "node-1", "dup-node2-v1", 1, 25, base.Add(1*time.Minute))
		first := h.nodes["node-1"].eng.State.Value
		h.deliverSingleContribution(t, "node-2", "node-1", "dup-node2-v1", 1, 25, base.Add(1*time.Minute))
		second := h.nodes["node-1"].eng.State.Value

		if math.Abs(first-second) > 1e-9 {
			t.Fatalf("duplicate update non idempotente: first=%v second=%v", first, second)
		}
		if math.Abs(second-35) > 1e-9 {
			t.Fatalf("somma inattesa dopo duplicate update: got=%v want=35", second)
		}
	})

	t.Run("out-of-order", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-3"})
		h.setLocalContribution("node-1", 10)
		h.setLocalContribution("node-3", 30)

		h.deliverSingleContribution(t, "node-3", "node-1", "node3-v5", 5, 35, base.Add(2*time.Minute))
		afterNew := h.nodes["node-1"].eng.State.Value
		h.deliverSingleContribution(t, "node-3", "node-1", "node3-v4-stale", 4, 5, base.Add(3*time.Minute))
		afterStale := h.nodes["node-1"].eng.State.Value

		if math.Abs(afterNew-afterStale) > 1e-9 {
			t.Fatalf("messaggio stale ha alterato la somma: new=%v stale=%v", afterNew, afterStale)
		}
		if math.Abs(afterStale-45) > 1e-9 {
			t.Fatalf("somma inattesa dopo out-of-order: got=%v want=45", afterStale)
		}
	})

	t.Run("nodo lento con ritardo ragionevole", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-2", "node-4"})
		h.setLocalContribution("node-1", 10)
		h.setLocalContribution("node-2", 20)
		h.setLocalContribution("node-4", 40)

		// Prima un update veloce: il nodo lento non è ancora arrivato.
		h.deliverSingleContribution(t, "node-1", "node-2", "node1-v1", 1, 10, base)
		if got := h.nodes["node-2"].eng.State.Value; math.Abs(got-30) > 1e-9 {
			t.Fatalf("baseline inattesa senza nodo lento: got=%v want=30", got)
		}

		// Poi update ritardato (250ms logici): nessuno sleep reale, quindi test deterministico.
		h.deliverSingleContribution(t, "node-4", "node-2", "node4-v3-delayed", 3, 40, base.Add(250*time.Millisecond))
		if got := h.nodes["node-2"].eng.State.Value; math.Abs(got-70) > 1e-9 {
			t.Fatalf("update del nodo lento non applicato: got=%v want=70", got)
		}
	})

	t.Run("valori estremi overflow con saturazione", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-a", "node-b"})
		h.setLocalContribution("node-a", math.MaxFloat64)
		h.setLocalContribution("node-b", math.MaxFloat64)

		h.deliverSingleContribution(t, "node-b", "node-a", "overflow-b-to-a", 1, math.MaxFloat64, base.Add(5*time.Minute))

		state := h.nodes["node-a"].eng.State
		if state.Value != math.MaxFloat64 {
			t.Fatalf("policy overflow non rispettata: got=%v want=%v", state.Value, math.MaxFloat64)
		}
		if state.AggregationData.Sum == nil || !state.AggregationData.Sum.Overflowed {
			t.Fatalf("flag overflow non impostato")
		}
	})
}
