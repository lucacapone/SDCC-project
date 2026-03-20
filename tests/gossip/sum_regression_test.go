package gossip

import (
	"math"
	"testing"
	"time"

	shared "sdcc-project/internal/types"
)

// TestSumRegressionConNuoveAggregazioni verifica che il merge sum resti invariato con nuove aggregazioni registrate.
func TestSumRegressionConNuoveAggregazioni(t *testing.T) {
	base := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "sum",
		Value:           10,
		Round:           1,
		VersionCounter:  1,
		UpdatedAt:       base,
		AggregationData: shared.AggregationState{Sum: &shared.SumState{
			Contributions: map[shared.NodeID]float64{"node-1": 10},
			Versions:      map[shared.NodeID]shared.StateVersionStamp{"node-1": {Counter: 1}},
		}},
	}
	local.EnsureMergeMetadata()

	fresh := shared.GossipMessage{
		MessageID:    "sum-node-2-v2",
		OriginNode:   "node-2",
		SentAt:       base.Add(time.Minute),
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: 2},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "sum",
			Value:           25,
			Round:           2,
			VersionCounter:  2,
			UpdatedAt:       base.Add(time.Minute),
			AggregationData: shared.AggregationState{Sum: &shared.SumState{
				Contributions: map[shared.NodeID]float64{"node-2": 25},
				Versions:      map[shared.NodeID]shared.StateVersionStamp{"node-2": {Counter: 2}},
			}},
		},
	}

	applied := applyRemote(local, fresh)
	if applied.Status != MergeApplied {
		t.Fatalf("merge fresco non applicato: status=%s reason=%s", applied.Status, applied.Reason)
	}
	if math.Abs(applied.State.Value-35) > 1e-9 {
		t.Fatalf("semantica sum alterata: got=%v want=35", applied.State.Value)
	}

	duplicate := applyRemote(applied.State, fresh)
	if duplicate.Status != MergeSkipped || duplicate.Reason != "duplicate_message_id" {
		t.Fatalf("duplicate sum non deduplicato: status=%s reason=%s", duplicate.Status, duplicate.Reason)
	}
	if math.Abs(duplicate.State.Value-35) > 1e-9 {
		t.Fatalf("duplicate ha alterato la somma: got=%v want=35", duplicate.State.Value)
	}

	stale := fresh
	stale.MessageID = "sum-node-2-v1-stale"
	stale.StateVersion = shared.StateVersionStamp{Counter: 1}
	stale.State.Round = 1
	stale.State.VersionCounter = 1
	stale.State.Value = 5
	stale.State.AggregationData.Sum.Contributions["node-2"] = 5
	stale.State.AggregationData.Sum.Versions["node-2"] = shared.StateVersionStamp{Counter: 1}

	staleResult := applyRemote(duplicate.State, stale)
	if staleResult.Status != MergeSkipped {
		t.Fatalf("messaggio stale non scartato: status=%s reason=%s", staleResult.Status, staleResult.Reason)
	}
	if math.Abs(staleResult.State.Value-35) > 1e-9 {
		t.Fatalf("messaggio stale ha alterato la somma: got=%v want=35", staleResult.State.Value)
	}
}
