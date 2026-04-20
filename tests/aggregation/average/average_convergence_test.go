package average_test

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

// deterministicTransport è uno stub transport con consegna sincrona in-memory.
type deterministicTransport struct {
	handler transport.MessageHandler
}

// Start registra l'handler di ricezione.
func (d *deterministicTransport) Start(_ context.Context, h transport.MessageHandler) error {
	d.handler = h
	return nil
}

// Send non usa la rete reale nei test di convergenza.
func (d *deterministicTransport) Send(context.Context, string, []byte) error { return nil }

// Close non richiede teardown reale nello stub.
func (d *deterministicTransport) Close() error { return nil }

// inject consegna direttamente il payload all'handler.
func (d *deterministicTransport) inject(ctx context.Context, payload []byte) error {
	if d.handler == nil {
		return fmt.Errorf("handler non inizializzato")
	}
	return d.handler(ctx, payload)
}

// testNode raggruppa engine e transport fake del nodo.
type testNode struct {
	eng *gossip.Engine
	tr  *deterministicTransport
}

// testHarness espone API deterministiche per setup/consegna messaggi.
type testHarness struct {
	nodes map[shared.NodeID]*testNode
}

// newTestHarness costruisce nodi average con ticker molto lento per evitare round automatici.
func newTestHarness(t *testing.T, ids []shared.NodeID) *testHarness {
	t.Helper()

	h := &testHarness{nodes: make(map[shared.NodeID]*testNode, len(ids))}
	for _, id := range ids {
		tr := &deterministicTransport{}
		eng := gossip.NewEngine(string(id), "average", tr, membership.NewSet(), slog.Default(), nil, 24*time.Hour, 2)
		eng.State.EnsureMergeMetadata()
		eng.State.EnsureAverageMetadata()

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

// setLocalContribution imposta contributo locale iniziale (sum/count) del nodo.
func (h *testHarness) setLocalContribution(id shared.NodeID, sum float64, count uint64) {
	n := h.nodes[id]
	n.eng.State.NodeID = id
	n.eng.State.AggregationType = "average"
	n.eng.State.LocalValue = sum
	n.eng.State.Round = 0
	n.eng.State.VersionCounter = 0
	n.eng.State.UpdatedAt = time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)
	n.eng.State.AggregationData.Average.Contributions[id] = shared.AverageContribution{Sum: sum, Count: count}
	n.eng.State.AggregationData.Average.Versions[id] = shared.StateVersionStamp{Counter: 0}
	n.eng.State.Value = safeAverage(n.eng.State.AggregationData.Average.Contributions)
}

// deliverContribution invia un singolo contributo average dal nodo from al nodo to.
func (h *testHarness) deliverContribution(t *testing.T, from, to shared.NodeID, messageID shared.MessageID, version shared.StateVersion, sum float64, count uint64, sentAt time.Time) {
	t.Helper()

	msg := shared.GossipMessage{
		MessageID:    messageID,
		OriginNode:   from,
		SentAt:       sentAt,
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: version},
		State: shared.GossipState{
			NodeID:          from,
			AggregationType: "average",
			Value:           valueForMessage(sum, count),
			Round:           version,
			VersionCounter:  version,
			UpdatedAt:       sentAt,
			AggregationData: shared.AggregationState{Average: &shared.AverageState{
				Contributions: map[shared.NodeID]shared.AverageContribution{from: {Sum: sum, Count: count}},
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

// assertNodeValue verifica il valore medio finale calcolato dal nodo.
func (h *testHarness) assertNodeValue(t *testing.T, id shared.NodeID, want float64) {
	t.Helper()
	got := h.nodes[id].eng.State.Value
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("valore inatteso su %s: got=%v want=%v", id, got, want)
	}
}

// TestAverageConvergence verifica convergenza robusta average su scenari distribuiti e edge case.
func TestAverageConvergence(t *testing.T) {
	base := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	t.Run("convergenza multi-nodo", func(t *testing.T) {
		ids := []shared.NodeID{"node-1", "node-2", "node-3"}
		h := newTestHarness(t, ids)
		h.setLocalContribution("node-1", 10, 1)
		h.setLocalContribution("node-2", 30, 1)
		h.setLocalContribution("node-3", 50, 1)

		versionByReceiver := map[shared.NodeID]shared.StateVersion{}
		for _, to := range ids {
			for _, from := range ids {
				if from == to {
					continue
				}
				versionByReceiver[to] += 10
				contribution := h.nodes[from].eng.State.AggregationData.Average.Contributions[from]
				h.deliverContribution(t, from, to, shared.MessageID(fmt.Sprintf("%s-to-%s-v%d", from, to, versionByReceiver[to])), versionByReceiver[to], contribution.Sum, contribution.Count, base)
			}
		}

		for _, id := range ids {
			h.assertNodeValue(t, id, 30)
		}
		for _, id := range ids {
			contribution := h.nodes[id].eng.State.AggregationData.Average.Contributions[id]
			if math.Abs(contribution.Sum-h.nodes[id].eng.State.LocalValue) > 1e-9 || contribution.Count != 1 {
				t.Fatalf("contributo locale driftato su %s: got=%+v local=%v", id, contribution, h.nodes[id].eng.State.LocalValue)
			}
		}
	})

	t.Run("duplicate update", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-2"})
		h.setLocalContribution("node-1", 10, 1)
		h.setLocalContribution("node-2", 40, 1)

		h.deliverContribution(t, "node-2", "node-1", "dup-node2-v1", 1, 40, 1, base.Add(1*time.Minute))
		first := h.nodes["node-1"].eng.State.Value
		h.deliverContribution(t, "node-2", "node-1", "dup-node2-v1", 1, 40, 1, base.Add(1*time.Minute))
		second := h.nodes["node-1"].eng.State.Value

		if math.Abs(first-second) > 1e-9 {
			t.Fatalf("duplicate update non idempotente: first=%v second=%v", first, second)
		}
		if math.Abs(second-25) > 1e-9 {
			t.Fatalf("media inattesa dopo duplicate update: got=%v want=25", second)
		}
	})

	t.Run("out-of-order", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-3"})
		h.setLocalContribution("node-1", 10, 1)
		h.setLocalContribution("node-3", 30, 1)

		h.deliverContribution(t, "node-3", "node-1", "node3-v5", 5, 50, 2, base.Add(2*time.Minute))
		afterNew := h.nodes["node-1"].eng.State.Value
		h.deliverContribution(t, "node-3", "node-1", "node3-v4-stale", 4, 5, 1, base.Add(3*time.Minute))
		afterStale := h.nodes["node-1"].eng.State.Value

		if math.Abs(afterNew-afterStale) > 1e-9 {
			t.Fatalf("messaggio stale ha alterato la media: new=%v stale=%v", afterNew, afterStale)
		}
		if math.Abs(afterStale-20) > 1e-9 {
			t.Fatalf("media inattesa dopo out-of-order: got=%v want=20", afterStale)
		}
	})

	t.Run("nodo lento", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-2", "node-4"})
		h.setLocalContribution("node-1", 10, 1)
		h.setLocalContribution("node-2", 20, 1)
		h.setLocalContribution("node-4", 40, 1)

		h.deliverContribution(t, "node-1", "node-2", "node1-v1", 1, 10, 1, base)
		if got := h.nodes["node-2"].eng.State.Value; math.Abs(got-15) > 1e-9 {
			t.Fatalf("baseline inattesa senza nodo lento: got=%v want=15", got)
		}

		h.deliverContribution(t, "node-4", "node-2", "node4-v3-delayed", 3, 40, 1, base.Add(250*time.Millisecond))
		if got := h.nodes["node-2"].eng.State.Value; math.Abs(got-(70.0/3.0)) > 1e-9 {
			t.Fatalf("update del nodo lento non applicato: got=%v want=%v", got, 70.0/3.0)
		}
	})

	t.Run("casi edge divisione per zero e stato vuoto", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-a"})
		h.setLocalContribution("node-a", 0, 0)
		if got := h.nodes["node-a"].eng.State.Value; got != 0 {
			t.Fatalf("count zero dovrebbe produrre media zero: got=%v", got)
		}

		h.deliverContribution(t, "node-a", "node-a", "self-empty-v1", 1, 0, 0, base.Add(10*time.Minute))
		if got := h.nodes["node-a"].eng.State.Value; got != 0 {
			t.Fatalf("stato vuoto dovrebbe restare a zero: got=%v", got)
		}
	})
}

// TestAverageRoundDoesNotDriftLocalContribution congela la regressione in cui round successivi
// riscrivevano il contributo locale del nodo con la media corrente del cluster.
func TestAverageRoundDoesNotDriftLocalContribution(t *testing.T) {
	h := newTestHarness(t, []shared.NodeID{"node-1"})
	h.setLocalContribution("node-1", 10, 1)

	n := h.nodes["node-1"]
	n.eng.State.AggregationData.Average.Contributions["node-2"] = shared.AverageContribution{Sum: 30, Count: 1}
	n.eng.State.AggregationData.Average.Contributions["node-3"] = shared.AverageContribution{Sum: 50, Count: 1}
	n.eng.State.AggregationData.Average.Versions["node-2"] = shared.StateVersionStamp{Counter: 1}
	n.eng.State.AggregationData.Average.Versions["node-3"] = shared.StateVersionStamp{Counter: 1}
	n.eng.State.Value = 30

	for round := 0; round < 4; round++ {
		n.eng.RoundOnce(context.Background())
	}

	localContribution := n.eng.State.AggregationData.Average.Contributions["node-1"]
	if math.Abs(localContribution.Sum-10) > 1e-9 || localContribution.Count != 1 {
		t.Fatalf("il contributo locale e' driftato dopo round multipli: got=%+v", localContribution)
	}
	if math.Abs(n.eng.State.Value-30) > 1e-9 {
		t.Fatalf("la media cluster attesa non e' stata preservata: got=%v want=30", n.eng.State.Value)
	}
}

// safeAverage calcola la media ignorando contributi con count zero.
func safeAverage(contributions map[shared.NodeID]shared.AverageContribution) float64 {
	totalSum := 0.0
	totalCount := uint64(0)
	for _, contribution := range contributions {
		if contribution.Count == 0 {
			continue
		}
		totalSum += contribution.Sum
		totalCount += contribution.Count
	}
	if totalCount == 0 {
		return 0
	}
	return totalSum / float64(totalCount)
}

// valueForMessage valorizza State.Value in modo coerente con sum/count del contributo.
func valueForMessage(sum float64, count uint64) float64 {
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
