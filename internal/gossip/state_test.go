package gossip

import (
	"testing"
	"time"

	shared "sdcc-project/internal/types"
)

func TestApplyRemote_DeduplicaMessageID(t *testing.T) {
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "average",
		Value:           10,
		Round:           2,
		VersionCounter:  2,
		UpdatedAt:       time.Now().UTC(),
	}
	msg := shared.GossipMessage{
		Envelope: shared.MessageEnvelope{MessageID: "node-2-2", SenderNodeID: "node-2"},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "average",
			Value:           30,
			Round:           3,
			VersionCounter:  3,
		},
	}

	first := applyRemote(local, msg)
	second := applyRemote(first.State, msg)

	if second.Status != MergeSkipped || second.Reason != "duplicate_message_id" {
		t.Fatalf("atteso skip duplicate, ottenuto status=%s reason=%s", second.Status, second.Reason)
	}
	if second.State.Value != first.State.Value {
		t.Fatalf("idempotenza violata su duplicato: first=%v second=%v", first.State.Value, second.State.Value)
	}
}

func TestApplyRemote_SameVersionSamePayloadNoOp(t *testing.T) {
	now := time.Now().UTC()
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "average",
		Value:           20,
		Round:           4,
		VersionCounter:  4,
		UpdatedAt:       now,
	}
	msg := shared.GossipMessage{
		Envelope: shared.MessageEnvelope{MessageID: "node-2-4", SenderNodeID: "node-2"},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "average",
			Value:           20,
			Round:           4,
			VersionCounter:  4,
			UpdatedAt:       now,
		},
	}

	res := applyRemote(local, msg)
	if res.Status != MergeSkipped || res.Reason != "same_version_same_payload" {
		t.Fatalf("atteso no-op stessa versione/payload, ottenuto status=%s reason=%s", res.Status, res.Reason)
	}
}

func TestApplyRemote_OlderVersionDrop(t *testing.T) {
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "average",
		Value:           40,
		Round:           8,
		VersionCounter:  8,
		UpdatedAt:       time.Now().UTC(),
	}
	msg := shared.GossipMessage{
		Envelope: shared.MessageEnvelope{MessageID: "node-2-3", SenderNodeID: "node-2"},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "average",
			Value:           5,
			Round:           3,
			VersionCounter:  3,
		},
	}

	res := applyRemote(local, msg)
	if res.Status != MergeSkipped || res.Reason != "older_version" {
		t.Fatalf("atteso drop versione vecchia, ottenuto status=%s reason=%s", res.Status, res.Reason)
	}
	if res.State.Value != local.Value {
		t.Fatalf("valore locale alterato da update vecchio")
	}
}

func TestApplyRemote_ConflictSameVersionDifferentPayload(t *testing.T) {
	local := shared.GossipState{
		NodeID:          "node-1",
		AggregationType: "average",
		Value:           10,
		Round:           5,
		VersionCounter:  5,
		UpdatedAt:       time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
	}
	msg := shared.GossipMessage{
		Envelope: shared.MessageEnvelope{MessageID: "node-2-5", SenderNodeID: "node-2"},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "average",
			Value:           99,
			Round:           5,
			VersionCounter:  5,
			UpdatedAt:       time.Date(2026, 3, 5, 11, 0, 0, 0, time.UTC),
		},
	}

	res := applyRemote(local, msg)
	if res.Status != MergeConflict || res.Reason != "same_version_different_payload" {
		t.Fatalf("atteso conflitto, ottenuto status=%s reason=%s", res.Status, res.Reason)
	}
	if res.State.Value != 99 {
		t.Fatalf("risoluzione conflitto non applicata: valore=%v", res.State.Value)
	}
}
