package gossip

import (
	"math"
	"time"

	"sdcc-project/internal/aggregation"
	shared "sdcc-project/internal/types"
)

// MergeStatus identifica l'esito del merge remoto per metriche/debug.
type MergeStatus string

const (
	MergeApplied  MergeStatus = "applied"
	MergeSkipped  MergeStatus = "skipped"
	MergeConflict MergeStatus = "conflict"
)

// MergeResult espone esito e motivazione del merge remoto.
type MergeResult struct {
	State  shared.GossipState
	Status MergeStatus
	Reason string
}

// applyRemote applica merge idempotente con deduplica, filtro out-of-order e gestione conflitti.
func applyRemote(local shared.GossipState, msg shared.GossipMessage) MergeResult {
	local.EnsureMergeMetadata()

	if _, seen := local.SeenMessageIDs[msg.MessageID]; seen {
		return MergeResult{State: local, Status: MergeSkipped, Reason: "duplicate_message_id"}
	}

	if local.AggregationType != "" && msg.State.AggregationType != "" && local.AggregationType != msg.State.AggregationType {
		local.SeenMessageIDs[msg.MessageID] = struct{}{}
		return MergeResult{State: local, Status: MergeConflict, Reason: "aggregation_type_mismatch"}
	}

	remoteVersion := normalizeMessageVersion(msg)
	localVersion := normalizeVersion(local)
	lastSeen, ok := local.LastSeenVersionByNode[msg.OriginNode]
	if ok && compareVersion(remoteVersion, lastSeen) < 0 {
		local.SeenMessageIDs[msg.MessageID] = struct{}{}
		return MergeResult{State: local, Status: MergeSkipped, Reason: "out_of_order_stale"}
	}

	cmp := compareVersion(remoteVersion, localVersion)
	samePayload := samePayload(local, msg.State)

	switch {
	case cmp < 0:
		local.SeenMessageIDs[msg.MessageID] = struct{}{}
		local.LastSeenVersionByNode[msg.OriginNode] = maxVersion(local.LastSeenVersionByNode[msg.OriginNode], remoteVersion)
		return MergeResult{State: local, Status: MergeSkipped, Reason: "older_version"}
	case cmp == 0 && samePayload:
		local.SeenMessageIDs[msg.MessageID] = struct{}{}
		local.LastSeenVersionByNode[msg.OriginNode] = maxVersion(local.LastSeenVersionByNode[msg.OriginNode], remoteVersion)
		return MergeResult{State: local, Status: MergeSkipped, Reason: "same_version_same_payload"}
	case cmp == 0 && !samePayload:
		local.SeenMessageIDs[msg.MessageID] = struct{}{}
		local.LastSeenVersionByNode[msg.OriginNode] = maxVersion(local.LastSeenVersionByNode[msg.OriginNode], remoteVersion)
		if preferRemoteOnConflict(msg, local) {
			local = adoptRemote(local, msg)
		}
		return MergeResult{State: local, Status: MergeConflict, Reason: "same_version_different_payload"}
	}

	local.SeenMessageIDs[msg.MessageID] = struct{}{}
	local.LastSeenVersionByNode[msg.OriginNode] = maxVersion(local.LastSeenVersionByNode[msg.OriginNode], remoteVersion)
	local.Value = mergeAggregationValue(local, msg.State)
	local.UpdatedAt = time.Now().UTC()
	local.Round = maxCounter(local.Round, msg.State.Round) + 1
	local.VersionEpoch = maxEpoch(local.VersionEpoch, msg.State.VersionEpoch)
	local.VersionCounter = maxCounter(local.VersionCounter, msg.State.VersionCounter) + 1
	local.LastMessageID = msg.MessageID
	local.LastSenderNodeID = msg.OriginNode
	return MergeResult{State: local, Status: MergeApplied, Reason: "remote_newer_version"}
}

func mergeAggregationValue(local, remote shared.GossipState) float64 {
	aggregationType := local.AggregationType
	if aggregationType == "" {
		aggregationType = remote.AggregationType
	}
	algo, err := aggregation.Factory(aggregationType)
	if err != nil {
		return (local.Value + remote.Value) / 2
	}
	return algo.Merge(local.Value, remote.Value)
}

func adoptRemote(local shared.GossipState, msg shared.GossipMessage) shared.GossipState {
	local.Value = msg.State.Value
	local.Round = maxCounter(local.Round, msg.State.Round)
	local.VersionEpoch = maxEpoch(local.VersionEpoch, msg.State.VersionEpoch)
	local.VersionCounter = maxCounter(local.VersionCounter, msg.State.VersionCounter)
	local.UpdatedAt = msg.State.UpdatedAt
	local.LastMessageID = msg.MessageID
	local.LastSenderNodeID = msg.OriginNode
	return local
}

func samePayload(local, remote shared.GossipState) bool {
	return local.AggregationType == remote.AggregationType && math.Abs(local.Value-remote.Value) < 1e-9
}

func preferRemoteOnConflict(msg shared.GossipMessage, local shared.GossipState) bool {
	if msg.State.UpdatedAt.After(local.UpdatedAt) {
		return true
	}
	if msg.State.UpdatedAt.Before(local.UpdatedAt) {
		return false
	}
	if msg.OriginNode > local.NodeID {
		return true
	}
	if msg.OriginNode < local.NodeID {
		return false
	}
	return msg.MessageID > local.LastMessageID
}

func normalizeVersion(state shared.GossipState) shared.StateVersionStamp {
	epoch := state.VersionEpoch
	counter := state.VersionCounter
	if counter == 0 && state.Round > 0 {
		counter = state.Round
	}
	return shared.StateVersionStamp{Epoch: epoch, Counter: counter}
}

func normalizeMessageVersion(msg shared.GossipMessage) shared.StateVersionStamp {
	if msg.StateVersion != (shared.StateVersionStamp{}) {
		return msg.StateVersion
	}
	return normalizeVersion(msg.State)
}

func compareVersion(a, b shared.StateVersionStamp) int {
	if a.Epoch < b.Epoch {
		return -1
	}
	if a.Epoch > b.Epoch {
		return 1
	}
	if a.Counter < b.Counter {
		return -1
	}
	if a.Counter > b.Counter {
		return 1
	}
	return 0
}

func maxVersion(a, b shared.StateVersionStamp) shared.StateVersionStamp {
	if compareVersion(a, b) >= 0 {
		return a
	}
	return b
}

func maxCounter(a, b shared.StateVersion) shared.StateVersion {
	if a >= b {
		return a
	}
	return b
}

func maxEpoch(a, b uint64) uint64 {
	if a >= b {
		return a
	}
	return b
}
