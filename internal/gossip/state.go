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
	local = mergeAggregationState(local, msg.State)
	local.UpdatedAt = time.Now().UTC()
	local.Round = maxCounter(local.Round, msg.State.Round) + 1
	local.VersionEpoch = maxEpoch(local.VersionEpoch, msg.State.VersionEpoch)
	local.VersionCounter = maxCounter(local.VersionCounter, msg.State.VersionCounter) + 1
	local.LastMessageID = msg.MessageID
	local.LastSenderNodeID = msg.OriginNode
	return MergeResult{State: local, Status: MergeApplied, Reason: "remote_newer_version"}
}

// mergeAggregationState applica la strategia di merge in base al tipo aggregazione.
func mergeAggregationState(local, remote shared.GossipState) shared.GossipState {
	aggregationType := local.AggregationType
	if aggregationType == "" {
		aggregationType = remote.AggregationType
	}
	if aggregationType == "sum" {
		return mergeSumState(local, remote)
	}
	local.Value = mergeAggregationValue(local, remote)
	return local
}

// mergeAggregationValue fonde valori numerici per aggregazioni non specializzate.
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

// mergeSumState implementa merge deterministico e idempotente su contributi per nodo.
func mergeSumState(local, remote shared.GossipState) shared.GossipState {
	local.EnsureSumMetadata()
	ensureIncomingSumMetadata(&remote)

	for nodeID, remoteVersion := range remote.AggregationData.Sum.Versions {
		localVersion, exists := local.AggregationData.Sum.Versions[nodeID]
		if exists && compareVersion(remoteVersion, localVersion) <= 0 {
			continue
		}
		local.AggregationData.Sum.Versions[nodeID] = remoteVersion
		local.AggregationData.Sum.Contributions[nodeID] = remote.AggregationData.Sum.Contributions[nodeID]
	}

	if remote.NodeID != "" {
		remoteContributionVersion := normalizeVersion(remote)
		localContributionVersion := local.AggregationData.Sum.Versions[remote.NodeID]
		if compareVersion(remoteContributionVersion, localContributionVersion) > 0 {
			local.AggregationData.Sum.Versions[remote.NodeID] = remoteContributionVersion
			local.AggregationData.Sum.Contributions[remote.NodeID] = remote.Value
		}
	}

	if remote.AggregationData.Sum.Overflowed {
		local.AggregationData.Sum.Overflowed = true
	}
	local.Value, local.AggregationData.Sum.Overflowed = sumWithSaturation(local.AggregationData.Sum.Contributions, local.AggregationData.Sum.Overflowed)
	return local
}

// ensureIncomingSumMetadata rende compatibili i messaggi legacy senza metadati sum.
func ensureIncomingSumMetadata(state *shared.GossipState) {
	if state.AggregationType != "sum" {
		return
	}
	state.EnsureSumMetadata()
	if state.NodeID == "" {
		return
	}
	version := normalizeVersion(*state)
	knownVersion, ok := state.AggregationData.Sum.Versions[state.NodeID]
	if !ok || compareVersion(version, knownVersion) > 0 {
		state.AggregationData.Sum.Versions[state.NodeID] = version
		state.AggregationData.Sum.Contributions[state.NodeID] = state.Value
	}
}

// sumWithSaturation somma i contributi saturando a +/- MaxFloat64 in caso di overflow.
func sumWithSaturation(contributions map[shared.NodeID]float64, alreadyOverflowed bool) (float64, bool) {
	total := 0.0
	overflowed := alreadyOverflowed
	for _, value := range contributions {
		if value > 0 && total > math.MaxFloat64-value {
			return math.MaxFloat64, true
		}
		if value < 0 && total < -math.MaxFloat64-value {
			return -math.MaxFloat64, true
		}
		next := total + value
		if math.IsInf(next, 1) || next > math.MaxFloat64 {
			return math.MaxFloat64, true
		}
		if math.IsInf(next, -1) || next < -math.MaxFloat64 {
			return -math.MaxFloat64, true
		}
		total = next
	}
	if overflowed && total > 0 {
		return math.MaxFloat64, true
	}
	if overflowed && total < 0 {
		return -math.MaxFloat64, true
	}
	return total, overflowed
}

func adoptRemote(local shared.GossipState, msg shared.GossipMessage) shared.GossipState {
	local.Value = msg.State.Value
	local.AggregationData = msg.State.AggregationData
	local.Round = maxCounter(local.Round, msg.State.Round)
	local.VersionEpoch = maxEpoch(local.VersionEpoch, msg.State.VersionEpoch)
	local.VersionCounter = maxCounter(local.VersionCounter, msg.State.VersionCounter)
	local.UpdatedAt = msg.State.UpdatedAt
	local.LastMessageID = msg.MessageID
	local.LastSenderNodeID = msg.OriginNode
	return local
}

func samePayload(local, remote shared.GossipState) bool {
	if local.AggregationType != remote.AggregationType {
		return false
	}
	if local.AggregationType == "sum" {
		return sameSumPayload(local, remote)
	}
	return math.Abs(local.Value-remote.Value) < 1e-9
}

func sameSumPayload(local, remote shared.GossipState) bool {
	local.EnsureSumMetadata()
	ensureIncomingSumMetadata(&remote)
	if local.AggregationData.Sum.Overflowed != remote.AggregationData.Sum.Overflowed {
		return false
	}
	if len(local.AggregationData.Sum.Contributions) != len(remote.AggregationData.Sum.Contributions) {
		return false
	}
	for nodeID, localValue := range local.AggregationData.Sum.Contributions {
		remoteValue, ok := remote.AggregationData.Sum.Contributions[nodeID]
		if !ok || math.Abs(localValue-remoteValue) > 1e-9 {
			return false
		}
		if compareVersion(local.AggregationData.Sum.Versions[nodeID], remote.AggregationData.Sum.Versions[nodeID]) != 0 {
			return false
		}
	}
	return true
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
