package gossip

import (
	"math"
	"testing"
	"time"

	shared "sdcc-project/internal/types"
)

func TestMergeRules(t *testing.T) {
	base := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)

	t.Run("merge idempotente sullo stesso messaggio", func(t *testing.T) {
		local := fixtureState("node-1", 10, 4, base)
		msg := fixtureMessage("msg-node-2-v5", "node-2", 30, 5, base.Add(1*time.Minute))

		first := applyRemote(local, msg)
		second := applyRemote(first.State, msg)

		if first.Status != MergeApplied || first.Reason != "remote_newer_version" {
			t.Fatalf("primo merge inatteso: status=%s reason=%s", first.Status, first.Reason)
		}
		if second.Status != MergeSkipped || second.Reason != "duplicate_message_id" {
			t.Fatalf("secondo merge non idempotente: status=%s reason=%s", second.Status, second.Reason)
		}
		if math.Abs(first.State.Value-second.State.Value) > 1e-9 {
			t.Fatalf("idempotenza violata: first=%v second=%v", first.State.Value, second.State.Value)
		}
	})

	t.Run("duplicate message delivery", func(t *testing.T) {
		local := fixtureState("node-1", 20, 4, base)
		msg := fixtureMessage("dup-msg", "node-3", 40, 5, base.Add(2*time.Minute))

		merged := applyRemote(local, msg)
		duplicated := applyRemote(merged.State, msg)

		if duplicated.Status != MergeSkipped || duplicated.Reason != "duplicate_message_id" {
			t.Fatalf("duplicate delivery non deduplicata: status=%s reason=%s", duplicated.Status, duplicated.Reason)
		}
		if _, seen := duplicated.State.SeenMessageIDs[msg.MessageID]; !seen {
			t.Fatalf("message id non tracciato in deduplica")
		}
	})

	t.Run("out-of-order delivery", func(t *testing.T) {
		local := fixtureState("node-1", 50, 7, base)
		local.LastSeenVersionByNode = map[shared.NodeID]shared.StateVersionStamp{
			"node-2": {Counter: 7},
		}

		msg := fixtureMessage("msg-node-2-v6", "node-2", 5, 6, base.Add(3*time.Minute))
		res := applyRemote(local, msg)

		if res.Status != MergeSkipped || res.Reason != "out_of_order_stale" {
			t.Fatalf("out-of-order non gestito: status=%s reason=%s", res.Status, res.Reason)
		}
		if res.State.Value != local.Value {
			t.Fatalf("stato alterato da messaggio out-of-order: got=%v want=%v", res.State.Value, local.Value)
		}
	})

	// aggregation_type_mismatch verifica che un payload con aggregazione incompatibile
	// venga segnalato come conflitto senza mutare il valore locale, marcando solo il message id.
	t.Run("aggregation_type_mismatch", func(t *testing.T) {
		local := fixtureState("node-1", 50, 7, base)
		local.LastSeenVersionByNode = map[shared.NodeID]shared.StateVersionStamp{
			"node-2": {Counter: 6},
		}

		msg := fixtureMessage("msg-type-mismatch", "node-2", 99, 8, base.Add(4*time.Minute))
		msg.State.AggregationType = "sum"

		res := applyRemote(local, msg)

		if res.Status != MergeConflict || res.Reason != "aggregation_type_mismatch" {
			t.Fatalf("mismatch aggregazione non rilevato: status=%s reason=%s", res.Status, res.Reason)
		}
		if res.State.Value != local.Value {
			t.Fatalf("valore locale alterato da mismatch aggregazione: got=%v want=%v", res.State.Value, local.Value)
		}
		if _, seen := res.State.SeenMessageIDs[msg.MessageID]; !seen {
			t.Fatalf("message id non tracciato nel mismatch aggregazione")
		}
		if got := res.State.LastSeenVersionByNode[msg.OriginNode]; got != local.LastSeenVersionByNode[msg.OriginNode] {
			t.Fatalf("last seen alterato nel mismatch aggregazione: got=%+v want=%+v", got, local.LastSeenVersionByNode[msg.OriginNode])
		}
	})

	// same_version_same_payload congela il ramo di skip idempotente quando versione e payload coincidono.
	t.Run("same_version_same_payload", func(t *testing.T) {
		local := fixtureState("node-1", 21, 9, base)
		local.SeenMessageIDs = map[shared.MessageID]struct{}{"existing-msg": {}}
		local.LastSeenVersionByNode = map[shared.NodeID]shared.StateVersionStamp{
			"node-2": {Counter: 8},
		}

		msg := fixtureMessage("msg-same-payload", "node-2", local.Value, local.VersionCounter, base.Add(5*time.Minute))
		msg.State = local
		msg.State.SeenMessageIDs = nil
		msg.State.LastSeenVersionByNode = nil

		res := applyRemote(local, msg)

		if res.Status != MergeSkipped || res.Reason != "same_version_same_payload" {
			t.Fatalf("skip stessa versione/payload non rilevato: status=%s reason=%s", res.Status, res.Reason)
		}
		if res.State.Value != local.Value {
			t.Fatalf("valore locale alterato da payload identico: got=%v want=%v", res.State.Value, local.Value)
		}
		if _, seen := res.State.SeenMessageIDs[msg.MessageID]; !seen {
			t.Fatalf("message id non tracciato per stessa versione/payload")
		}
		if got := res.State.LastSeenVersionByNode[msg.OriginNode]; got != msg.StateVersion {
			t.Fatalf("last seen non aggiornato sulla versione attesa: got=%+v want=%+v", got, msg.StateVersion)
		}
	})

	// older_version verifica lo scarto di un messaggio più vecchio del locale ma non stale
	// rispetto all'ultima versione vista per il mittente.
	t.Run("older_version", func(t *testing.T) {
		local := fixtureState("node-1", 70, 9, base)
		local.LastSeenVersionByNode = map[shared.NodeID]shared.StateVersionStamp{
			"node-2": {Counter: 3},
		}

		msg := fixtureMessage("msg-older-version", "node-2", 10, 7, base.Add(6*time.Minute))

		res := applyRemote(local, msg)

		if res.Status != MergeSkipped || res.Reason != "older_version" {
			t.Fatalf("older version non gestita: status=%s reason=%s", res.Status, res.Reason)
		}
		if res.State.Value != local.Value {
			t.Fatalf("valore locale alterato da versione vecchia: got=%v want=%v", res.State.Value, local.Value)
		}
		if _, seen := res.State.SeenMessageIDs[msg.MessageID]; !seen {
			t.Fatalf("message id non tracciato per older version")
		}
		if got := res.State.LastSeenVersionByNode[msg.OriginNode]; got != msg.StateVersion {
			t.Fatalf("last seen inatteso per older version: got=%+v want=%+v", got, msg.StateVersion)
		}
	})

	t.Run("conflitto versione stato", func(t *testing.T) {
		local := fixtureState("node-1", 11, 8, base.Add(10*time.Minute))
		msg := fixtureMessage("msg-conflict", "node-2", 99, 8, base.Add(11*time.Minute))

		res := applyRemote(local, msg)

		if res.Status != MergeConflict || res.Reason != "same_version_different_payload" {
			t.Fatalf("conflitto versione non rilevato: status=%s reason=%s", res.Status, res.Reason)
		}
		if res.State.Value != 99 {
			t.Fatalf("tie-break conflitto non applicato: got=%v want=%v", res.State.Value, 99.0)
		}
	})

	t.Run("convergenza logica a parita di scambi", func(t *testing.T) {
		nodeA := fixtureState("node-a", 10, 1, base)
		nodeB := fixtureState("node-b", 30, 1, base)

		ab := fixtureMessage("a-to-b-v2", "node-a", nodeA.Value, 2, base.Add(20*time.Minute))
		ba := fixtureMessage("b-to-a-v2", "node-b", nodeB.Value, 2, base.Add(20*time.Minute))

		aAfter := applyRemote(nodeA, ba).State
		bAfter := applyRemote(nodeB, ab).State

		if math.Abs(aAfter.Value-bAfter.Value) > 1e-9 {
			t.Fatalf("nodi non convergenti con scambi simmetrici: a=%v b=%v", aAfter.Value, bAfter.Value)
		}
		if aAfter.VersionCounter != bAfter.VersionCounter {
			t.Fatalf("versioni divergenti dopo scambi simmetrici: a=%d b=%d", aAfter.VersionCounter, bAfter.VersionCounter)
		}
	})
}

func fixtureState(node shared.NodeID, value float64, version shared.StateVersion, updatedAt time.Time) shared.GossipState {
	return shared.GossipState{
		NodeID:          node,
		AggregationType: "average",
		Value:           value,
		Round:           version,
		VersionCounter:  version,
		UpdatedAt:       updatedAt,
		AggregationData: shared.AggregationState{Average: &shared.AverageState{
			Contributions: map[shared.NodeID]shared.AverageContribution{node: {Sum: value, Count: 1}},
			Versions:      map[shared.NodeID]shared.StateVersionStamp{node: {Counter: version}},
		}},
	}
}

func fixtureMessage(id shared.MessageID, origin shared.NodeID, value float64, version shared.StateVersion, sentAt time.Time) shared.GossipMessage {
	return shared.GossipMessage{
		MessageID:    id,
		OriginNode:   origin,
		SentAt:       sentAt,
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: version},
		State: shared.GossipState{
			NodeID:          origin,
			AggregationType: "average",
			Value:           value,
			Round:           version,
			VersionCounter:  version,
			UpdatedAt:       sentAt,
			AggregationData: shared.AggregationState{Average: &shared.AverageState{
				Contributions: map[shared.NodeID]shared.AverageContribution{origin: {Sum: value, Count: 1}},
				Versions:      map[shared.NodeID]shared.StateVersionStamp{origin: {Counter: version}},
			}},
		},
	}
}

func TestMergeSumIdempotenteConContributiPerNodo(t *testing.T) {
	base := time.Date(2026, 3, 16, 18, 0, 0, 0, time.UTC)
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "sum",
		Value:           10,
		Round:           2,
		VersionCounter:  2,
		UpdatedAt:       base,
		AggregationData: shared.AggregationState{Sum: &shared.SumState{
			Contributions: map[shared.NodeID]float64{"node-1": 10},
			Versions:      map[shared.NodeID]shared.StateVersionStamp{"node-1": {Counter: 2}},
		}},
	}
	msg := shared.GossipMessage{
		MessageID:    "sum-msg-1",
		OriginNode:   "node-2",
		SentAt:       base.Add(1 * time.Minute),
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: 3},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "sum",
			Value:           20,
			Round:           3,
			VersionCounter:  3,
			UpdatedAt:       base.Add(1 * time.Minute),
			AggregationData: shared.AggregationState{Sum: &shared.SumState{
				Contributions: map[shared.NodeID]float64{"node-2": 20},
				Versions:      map[shared.NodeID]shared.StateVersionStamp{"node-2": {Counter: 3}},
			}},
		},
	}

	first := applyRemote(local, msg)
	second := applyRemote(first.State, msg)
	if first.State.Value != 30 {
		t.Fatalf("somma inattesa dopo merge: got=%v want=30", first.State.Value)
	}
	if second.State.Value != 30 {
		t.Fatalf("somma non idempotente su duplicato: got=%v want=30", second.State.Value)
	}
}

func TestMergeSumOutOfOrderNonRegredisceContributo(t *testing.T) {
	base := time.Date(2026, 3, 16, 18, 10, 0, 0, time.UTC)
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "sum",
		Value:           35,
		Round:           5,
		VersionCounter:  5,
		UpdatedAt:       base,
		AggregationData: shared.AggregationState{Sum: &shared.SumState{
			Contributions: map[shared.NodeID]float64{"node-1": 10, "node-2": 25},
			Versions: map[shared.NodeID]shared.StateVersionStamp{
				"node-1": {Counter: 2},
				"node-2": {Counter: 5},
			},
		}},
	}
	msg := shared.GossipMessage{
		MessageID:    "sum-stale-node2",
		OriginNode:   "node-2",
		SentAt:       base.Add(2 * time.Minute),
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: 4},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "sum",
			Value:           5,
			Round:           4,
			VersionCounter:  4,
			UpdatedAt:       base.Add(2 * time.Minute),
			AggregationData: shared.AggregationState{Sum: &shared.SumState{
				Contributions: map[shared.NodeID]float64{"node-2": 5},
				Versions:      map[shared.NodeID]shared.StateVersionStamp{"node-2": {Counter: 4}},
			}},
		},
	}
	res := applyRemote(local, msg)
	if res.State.Value != 35 {
		t.Fatalf("contributo stale ha regredito somma: got=%v want=35", res.State.Value)
	}
}

func TestMergeSumOverflowSaturazione(t *testing.T) {
	base := time.Date(2026, 3, 16, 18, 20, 0, 0, time.UTC)
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "sum",
		Value:           math.MaxFloat64,
		Round:           2,
		VersionCounter:  2,
		UpdatedAt:       base,
		AggregationData: shared.AggregationState{Sum: &shared.SumState{
			Contributions: map[shared.NodeID]float64{"node-1": math.MaxFloat64},
			Versions:      map[shared.NodeID]shared.StateVersionStamp{"node-1": {Counter: 2}},
		}},
	}
	msg := shared.GossipMessage{
		MessageID:    "sum-overflow",
		OriginNode:   "node-2",
		SentAt:       base.Add(1 * time.Minute),
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: 3},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "sum",
			Value:           42,
			Round:           3,
			VersionCounter:  3,
			UpdatedAt:       base.Add(1 * time.Minute),
			AggregationData: shared.AggregationState{Sum: &shared.SumState{
				Contributions: map[shared.NodeID]float64{"node-2": math.MaxFloat64},
				Versions:      map[shared.NodeID]shared.StateVersionStamp{"node-2": {Counter: 3}},
			}},
		},
	}
	res := applyRemote(local, msg)
	if res.State.Value != math.MaxFloat64 {
		t.Fatalf("saturazione overflow non applicata: got=%v want=%v", res.State.Value, math.MaxFloat64)
	}
	if res.State.AggregationData.Sum == nil || !res.State.AggregationData.Sum.Overflowed {
		t.Fatalf("flag overflow non impostato")
	}
}

func TestMergeAverageContributiConvergentiPerNodo(t *testing.T) {
	base := time.Date(2026, 3, 16, 18, 30, 0, 0, time.UTC)
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "average",
		Value:           10,
		Round:           2,
		VersionCounter:  2,
		UpdatedAt:       base,
		AggregationData: shared.AggregationState{Average: &shared.AverageState{
			Contributions: map[shared.NodeID]shared.AverageContribution{
				"node-1": {Sum: 10, Count: 1},
			},
			Versions: map[shared.NodeID]shared.StateVersionStamp{
				"node-1": {Counter: 2},
			},
		}},
	}
	msg := shared.GossipMessage{
		MessageID:    "avg-msg-1",
		OriginNode:   "node-2",
		SentAt:       base.Add(1 * time.Minute),
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: 3},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "average",
			Value:           30,
			Round:           3,
			VersionCounter:  3,
			UpdatedAt:       base.Add(1 * time.Minute),
			AggregationData: shared.AggregationState{Average: &shared.AverageState{
				Contributions: map[shared.NodeID]shared.AverageContribution{
					"node-2": {Sum: 30, Count: 1},
				},
				Versions: map[shared.NodeID]shared.StateVersionStamp{
					"node-2": {Counter: 3},
				},
			}},
		},
	}

	first := applyRemote(local, msg)
	second := applyRemote(first.State, msg)
	if first.State.Value != 20 {
		t.Fatalf("media inattesa dopo merge: got=%v want=20", first.State.Value)
	}
	if second.State.Value != 20 {
		t.Fatalf("media non idempotente su duplicato: got=%v want=20", second.State.Value)
	}
}

func TestMergeAverageLegacySenzaMetadataCompatibile(t *testing.T) {
	base := time.Date(2026, 3, 16, 19, 0, 0, 0, time.UTC)
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "average",
		Value:           10,
		Round:           2,
		VersionCounter:  2,
		UpdatedAt:       base,
		AggregationData: shared.AggregationState{Average: &shared.AverageState{
			Contributions: map[shared.NodeID]shared.AverageContribution{"node-1": {Sum: 10, Count: 1}},
			Versions:      map[shared.NodeID]shared.StateVersionStamp{"node-1": {Counter: 2}},
		}},
	}
	msg := shared.GossipMessage{
		MessageID:    "avg-legacy",
		OriginNode:   "node-2",
		SentAt:       base.Add(1 * time.Minute),
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: 3},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "average",
			Value:           30,
			Round:           3,
			VersionCounter:  3,
			UpdatedAt:       base.Add(1 * time.Minute),
			AggregationData: shared.AggregationState{},
		},
	}

	res := applyRemote(local, msg)
	if res.State.Value != 20 {
		t.Fatalf("media legacy non ricostruita: got=%v want=20", res.State.Value)
	}
}

func TestMergeAverageNonReinferisceContributoDaRemoteValueQuandoMetadataCompleti(t *testing.T) {
	base := time.Date(2026, 3, 16, 19, 5, 0, 0, time.UTC)
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "average",
		Value:           10,
		Round:           2,
		VersionCounter:  2,
		UpdatedAt:       base,
		AggregationData: shared.AggregationState{Average: &shared.AverageState{
			Contributions: map[shared.NodeID]shared.AverageContribution{"node-1": {Sum: 10, Count: 1}},
			Versions:      map[shared.NodeID]shared.StateVersionStamp{"node-1": {Counter: 2}},
		}},
	}
	msg := shared.GossipMessage{
		MessageID:    "avg-complete-metadata",
		OriginNode:   "node-2",
		SentAt:       base.Add(1 * time.Minute),
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: 3},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "average",
			Value:           30,
			Round:           3,
			VersionCounter:  3,
			UpdatedAt:       base.Add(1 * time.Minute),
			AggregationData: shared.AggregationState{Average: &shared.AverageState{
				Contributions: map[shared.NodeID]shared.AverageContribution{
					"node-2": {Sum: 50, Count: 2},
				},
				Versions: map[shared.NodeID]shared.StateVersionStamp{
					"node-2": {Counter: 3},
				},
			}},
		},
	}

	res := applyRemote(local, msg)
	gotContribution := res.State.AggregationData.Average.Contributions["node-2"]
	if gotContribution != (shared.AverageContribution{Sum: 50, Count: 2}) {
		t.Fatalf("contributo remoto reinferito impropriamente: got=%+v", gotContribution)
	}
	if math.Abs(res.State.Value-20) > 1e-9 {
		t.Fatalf("media inattesa dopo merge con metadata completi: got=%v want=20", res.State.Value)
	}
}

func TestMergeMinMonotonoGestisceStatoVuotoELegacy(t *testing.T) {
	base := time.Date(2026, 3, 16, 19, 10, 0, 0, time.UTC)
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "min",
		Value:           0,
		Round:           0,
		VersionCounter:  0,
		UpdatedAt:       base,
		AggregationData: shared.AggregationState{},
	}
	msg := shared.GossipMessage{
		MessageID:    "min-legacy",
		OriginNode:   "node-2",
		SentAt:       base.Add(1 * time.Minute),
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: 1},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "min",
			Value:           7,
			Round:           1,
			VersionCounter:  1,
			UpdatedAt:       base.Add(1 * time.Minute),
			AggregationData: shared.AggregationState{},
		},
	}

	first := applyRemote(local, msg)
	if first.State.Value != 7 {
		t.Fatalf("merge min da stato vuoto inatteso: got=%v want=7", first.State.Value)
	}

	newer := msg
	newer.MessageID = "min-newer"
	newer.State.Value = 5
	newer.State.Round = 3
	newer.State.VersionCounter = 3
	newer.StateVersion = shared.StateVersionStamp{Counter: 3}
	second := applyRemote(first.State, newer)
	if second.State.Value != 5 {
		t.Fatalf("merge min monotono non aggiornato: got=%v want=5", second.State.Value)
	}
}

func TestMergeMaxMonotonoGestisceStatoVuotoELegacy(t *testing.T) {
	base := time.Date(2026, 3, 16, 19, 20, 0, 0, time.UTC)
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "max",
		Value:           0,
		Round:           0,
		VersionCounter:  0,
		UpdatedAt:       base,
		AggregationData: shared.AggregationState{},
	}
	msg := shared.GossipMessage{
		MessageID:    "max-legacy",
		OriginNode:   "node-2",
		SentAt:       base.Add(1 * time.Minute),
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: 1},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "max",
			Value:           7,
			Round:           1,
			VersionCounter:  1,
			UpdatedAt:       base.Add(1 * time.Minute),
			AggregationData: shared.AggregationState{},
		},
	}

	first := applyRemote(local, msg)
	if first.State.Value != 7 {
		t.Fatalf("merge max da stato vuoto inatteso: got=%v want=7", first.State.Value)
	}

	newer := msg
	newer.MessageID = "max-newer"
	newer.State.Value = 9
	newer.State.Round = 3
	newer.State.VersionCounter = 3
	newer.StateVersion = shared.StateVersionStamp{Counter: 3}
	second := applyRemote(first.State, newer)
	if second.State.Value != 9 {
		t.Fatalf("merge max monotono non aggiornato: got=%v want=9", second.State.Value)
	}
}
