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
	State               shared.GossipState
	Status              MergeStatus
	Reason              string
	EstimateBefore      float64
	EstimateAfter       float64
	MaxPreserved        bool
	UniqueContributions int
	NodeDecisions       map[shared.NodeID]string
}

// applyRemote applica merge idempotente con deduplica, filtro out-of-order e gestione conflitti.
func applyRemote(local shared.GossipState, msg shared.GossipMessage) MergeResult {
	local.EnsureMergeMetadata()
	estimateBefore := local.Value
	aggregationType := effectiveAggregationType(local.AggregationType, msg.State.AggregationType)

	if _, seen := local.SeenMessageIDs[msg.MessageID]; seen {
		return buildMergeResult(local, MergeSkipped, "duplicate_message_id", estimateBefore, msg.State.Value, nil, false)
	}

	if local.AggregationType != "" && msg.State.AggregationType != "" && local.AggregationType != msg.State.AggregationType {
		local.SeenMessageIDs[msg.MessageID] = struct{}{}
		return buildMergeResult(local, MergeConflict, "aggregation_type_mismatch", estimateBefore, msg.State.Value, nil, false)
	}

	remoteVersion := normalizeMessageVersion(msg)
	localVersion := normalizeVersion(local)
	lastSeen, ok := local.LastSeenVersionByNode[msg.OriginNode]
	if ok && compareVersion(remoteVersion, lastSeen) < 0 {
		local.SeenMessageIDs[msg.MessageID] = struct{}{}
		return buildMergeResult(local, MergeSkipped, "out_of_order_stale", estimateBefore, msg.State.Value, nil, false)
	}

	cmp := compareVersion(remoteVersion, localVersion)
	samePayload := samePayload(local, msg.State)

	if usesPerNodeMerge(local.AggregationType, msg.State.AggregationType) {
		local.SeenMessageIDs[msg.MessageID] = struct{}{}
		local.LastSeenVersionByNode[msg.OriginNode] = maxVersion(local.LastSeenVersionByNode[msg.OriginNode], remoteVersion)
		local, nodeDecisions := mergeAggregationState(local, msg.State)
		local.UpdatedAt = time.Now().UTC()
		local.Round = maxCounter(local.Round, msg.State.Round) + 1
		local.VersionEpoch = maxEpoch(local.VersionEpoch, msg.State.VersionEpoch)
		local.VersionCounter = maxCounter(local.VersionCounter, msg.State.VersionCounter) + 1
		local.LastMessageID = msg.MessageID
		local.LastSenderNodeID = msg.OriginNode
		return buildMergeResult(local, MergeApplied, "remote_contribution_merged", estimateBefore, msg.State.Value, nodeDecisions, false)
	}

	decision := resolveMergeDecision(local, msg, cmp, samePayload)
	switch decision.Status {
	case MergeSkipped:
		local.SeenMessageIDs[msg.MessageID] = struct{}{}
		local.LastSeenVersionByNode[msg.OriginNode] = maxVersion(local.LastSeenVersionByNode[msg.OriginNode], remoteVersion)
		return buildMergeResult(local, decision.Status, decision.Reason, estimateBefore, msg.State.Value, nil, false)
	case MergeConflict:
		local.SeenMessageIDs[msg.MessageID] = struct{}{}
		local.LastSeenVersionByNode[msg.OriginNode] = maxVersion(local.LastSeenVersionByNode[msg.OriginNode], remoteVersion)
		maxPreserved := false
		if aggregationType == "max" {
			local = mergeMaxState(local, msg.State)
			local.Value = math.Max(estimateBefore, msg.State.Value)
			maxPreserved = true
		} else if decision.AdoptRemote {
			local = adoptRemote(local, msg)
		}
		return buildMergeResult(local, decision.Status, decision.Reason, estimateBefore, msg.State.Value, nil, maxPreserved)
	}

	local.SeenMessageIDs[msg.MessageID] = struct{}{}
	local.LastSeenVersionByNode[msg.OriginNode] = maxVersion(local.LastSeenVersionByNode[msg.OriginNode], remoteVersion)
	local, nodeDecisions := mergeAggregationState(local, msg.State)
	local.UpdatedAt = time.Now().UTC()
	local.Round = maxCounter(local.Round, msg.State.Round) + 1
	local.VersionEpoch = maxEpoch(local.VersionEpoch, msg.State.VersionEpoch)
	local.VersionCounter = maxCounter(local.VersionCounter, msg.State.VersionCounter) + 1
	local.LastMessageID = msg.MessageID
	local.LastSenderNodeID = msg.OriginNode
	return buildMergeResult(local, MergeApplied, "remote_newer_version", estimateBefore, msg.State.Value, nodeDecisions, false)
}

// ApplyRemote espone il merge remoto per le suite esterne che validano il contratto del package gossip.
func ApplyRemote(local shared.GossipState, msg shared.GossipMessage) MergeResult {
	return applyRemote(local, msg)
}

// NormalizeStateVersion espone la normalizzazione della versione di stato per i test esterni.
func NormalizeStateVersion(state shared.GossipState) shared.StateVersionStamp {
	return normalizeVersion(state)
}

// mergeAggregationState applica la strategia di merge in base al tipo aggregazione.
func usesPerNodeMerge(localAggregationType, remoteAggregationType string) bool {
	aggregationType := localAggregationType
	if aggregationType == "" {
		aggregationType = remoteAggregationType
	}
	switch aggregationType {
	case "sum":
		return true
	default:
		return false
	}
}

func mergeAggregationState(local, remote shared.GossipState) (shared.GossipState, map[shared.NodeID]string) {
	aggregationType := local.AggregationType
	if aggregationType == "" {
		aggregationType = remote.AggregationType
	}
	switch aggregationType {
	case "sum":
		return mergeSumState(local, remote)
	case "average":
		return mergeAverageState(local, remote), nil
	case "min":
		return mergeMinState(local, remote), nil
	case "max":
		return mergeMaxState(local, remote), nil
	default:
		local.Value = mergeAggregationValue(local, remote)
		return local, nil
	}
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
func mergeSumState(local, remote shared.GossipState) (shared.GossipState, map[shared.NodeID]string) {
	local.EnsureSumMetadata()
	ensureIncomingSumMetadata(&remote)
	nodeDecisions := make(map[shared.NodeID]string)

	for nodeID, remoteContribution := range remote.AggregationData.Sum.Contributions {
		remoteVersion := remote.AggregationData.Sum.Versions[nodeID]
		localVersion, exists := local.AggregationData.Sum.Versions[nodeID]
		if !exists || compareVersion(remoteVersion, localVersion) > 0 {
			local.AggregationData.Sum.Versions[nodeID] = remoteVersion
			local.AggregationData.Sum.Contributions[nodeID] = remoteContribution
			nodeDecisions[nodeID] = "newer_version"
			continue
		}
		if exists && compareVersion(remoteVersion, localVersion) < 0 {
			nodeDecisions[nodeID] = "duplicate_ignored"
			continue
		}
		localContribution := local.AggregationData.Sum.Contributions[nodeID]
		if math.Abs(localContribution-remoteContribution) <= 1e-9 {
			nodeDecisions[nodeID] = "duplicate_ignored"
			continue
		}
		if remoteContribution > localContribution {
			local.AggregationData.Sum.Contributions[nodeID] = remoteContribution
			nodeDecisions[nodeID] = "tie_break"
			continue
		}
		nodeDecisions[nodeID] = "duplicate_ignored"
	}

	if remote.NodeID != "" {
		remoteContributionVersion := normalizeVersion(remote)
		localContributionVersion := local.AggregationData.Sum.Versions[remote.NodeID]
		if compareVersion(remoteContributionVersion, localContributionVersion) > 0 {
			local.AggregationData.Sum.Versions[remote.NodeID] = remoteContributionVersion
			local.AggregationData.Sum.Contributions[remote.NodeID] = remote.Value
			nodeDecisions[remote.NodeID] = "newer_version"
		} else if compareVersion(remoteContributionVersion, localContributionVersion) == 0 {
			localContribution := local.AggregationData.Sum.Contributions[remote.NodeID]
			if remote.Value > localContribution {
				local.AggregationData.Sum.Contributions[remote.NodeID] = remote.Value
				nodeDecisions[remote.NodeID] = "tie_break"
			} else if _, known := nodeDecisions[remote.NodeID]; !known {
				nodeDecisions[remote.NodeID] = "duplicate_ignored"
			}
		} else if _, known := nodeDecisions[remote.NodeID]; !known {
			nodeDecisions[remote.NodeID] = "duplicate_ignored"
		}
	}

	if remote.AggregationData.Sum.Overflowed {
		local.AggregationData.Sum.Overflowed = true
	}
	local.Value, local.AggregationData.Sum.Overflowed = sumWithSaturation(local.AggregationData.Sum.Contributions, local.AggregationData.Sum.Overflowed)
	return local, nodeDecisions
}

func buildMergeResult(state shared.GossipState, status MergeStatus, reason string, estimateBefore float64, remoteEstimate float64, nodeDecisions map[shared.NodeID]string, maxPreserved bool) MergeResult {
	uniqueContributions := 0
	if state.AggregationData.Sum != nil {
		uniqueContributions = len(state.AggregationData.Sum.Contributions)
	}
	return MergeResult{
		State:               state,
		Status:              status,
		Reason:              reason,
		EstimateBefore:      estimateBefore,
		EstimateAfter:       state.Value,
		MaxPreserved:        maxPreserved || (state.AggregationType == "max" && math.Abs(state.Value-math.Max(estimateBefore, remoteEstimate)) <= 1e-9),
		UniqueContributions: uniqueContributions,
		NodeDecisions:       nodeDecisions,
	}
}

type mergeDecision struct {
	Status      MergeStatus
	Reason      string
	AdoptRemote bool
}

// resolveMergeDecision classifica l'esito logico del merge senza applicare trasformazioni numeriche.
func resolveMergeDecision(local shared.GossipState, msg shared.GossipMessage, versionCompare int, samePayload bool) mergeDecision {
	switch {
	case versionCompare < 0:
		return mergeDecision{Status: MergeSkipped, Reason: "older_version"}
	case versionCompare == 0 && samePayload:
		return mergeDecision{Status: MergeSkipped, Reason: "same_version_same_payload"}
	case versionCompare == 0 && !samePayload:
		if samePayloadSemantically(local, msg.State) {
			return mergeDecision{Status: MergeSkipped, Reason: "same_version_semantically_equivalent"}
		}
		return mergeDecision{
			Status:      MergeConflict,
			Reason:      "same_version_different_payload",
			AdoptRemote: preferRemoteOnConflict(msg, local),
		}
	default:
		return mergeDecision{Status: MergeApplied, Reason: "remote_newer_version"}
	}
}

func effectiveAggregationType(localAggregationType, remoteAggregationType string) string {
	if localAggregationType != "" {
		return localAggregationType
	}
	return remoteAggregationType
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

// mergeAverageState implementa merge convergente per media via contributi per nodo (sum/count).
func mergeAverageState(local, remote shared.GossipState) shared.GossipState {
	local.EnsureAverageMetadata()
	ensureIncomingAverageMetadata(&remote)

	for nodeID, remoteVersion := range remote.AggregationData.Average.Versions {
		localVersion, exists := local.AggregationData.Average.Versions[nodeID]
		if exists && compareVersion(remoteVersion, localVersion) <= 0 {
			continue
		}
		local.AggregationData.Average.Versions[nodeID] = remoteVersion
		local.AggregationData.Average.Contributions[nodeID] = remote.AggregationData.Average.Contributions[nodeID]
	}

	local.Value = averageFromContributions(local.AggregationData.Average.Contributions)
	return local
}

// ensureIncomingAverageMetadata rende compatibili i messaggi legacy senza metadati average.
func ensureIncomingAverageMetadata(state *shared.GossipState) {
	if state.AggregationType != "average" {
		return
	}
	state.EnsureAverageMetadata()
	if state.NodeID == "" {
		return
	}
	// Se il payload espone gia' metadata average completi per il nodo remoto, il contributo
	// canonico e' quello serializzato in `aggregation_data.average` e non va re-inferito da `value`.
	if _, ok := state.AggregationData.Average.Contributions[state.NodeID]; ok {
		if _, versionKnown := state.AggregationData.Average.Versions[state.NodeID]; versionKnown {
			return
		}
	}
	version := normalizeVersion(*state)
	knownVersion, ok := state.AggregationData.Average.Versions[state.NodeID]
	if !ok || compareVersion(version, knownVersion) > 0 {
		state.AggregationData.Average.Versions[state.NodeID] = version
		if _, hasContribution := state.AggregationData.Average.Contributions[state.NodeID]; !hasContribution {
			state.AggregationData.Average.Contributions[state.NodeID] = shared.AverageContribution{Sum: state.Value, Count: 1}
		}
	}
}

// mergeMinState implementa merge monotono robusto per minimo con gestione stati legacy/vuoti.
func mergeMinState(local, remote shared.GossipState) shared.GossipState {
	local.EnsureMinMetadata()
	localInitialized := len(local.AggregationData.Min.Versions) > 0
	ensureIncomingMinMetadata(&remote)
	appliedRemote := false

	for nodeID, remoteVersion := range remote.AggregationData.Min.Versions {
		localVersion, exists := local.AggregationData.Min.Versions[nodeID]
		if exists && compareVersion(remoteVersion, localVersion) <= 0 {
			continue
		}
		local.AggregationData.Min.Versions[nodeID] = remoteVersion
		appliedRemote = true
	}

	if remote.NodeID != "" {
		remoteContributionVersion := normalizeVersion(remote)
		localContributionVersion := local.AggregationData.Min.Versions[remote.NodeID]
		if compareVersion(remoteContributionVersion, localContributionVersion) > 0 {
			local.AggregationData.Min.Versions[remote.NodeID] = remoteContributionVersion
			appliedRemote = true
		}
	}

	if !localInitialized {
		local.Value = remote.Value
		return local
	}
	if appliedRemote {
		local.Value = math.Min(local.Value, remote.Value)
	}
	return local
}

// ensureIncomingMinMetadata rende compatibili i messaggi legacy senza metadati min.
func ensureIncomingMinMetadata(state *shared.GossipState) {
	if state.AggregationType != "min" {
		return
	}
	state.EnsureMinMetadata()
	if state.NodeID == "" {
		return
	}
	version := normalizeVersion(*state)
	knownVersion, ok := state.AggregationData.Min.Versions[state.NodeID]
	if !ok || compareVersion(version, knownVersion) > 0 {
		state.AggregationData.Min.Versions[state.NodeID] = version
	}
}

// mergeMaxState implementa merge monotono robusto per massimo con gestione stati legacy/vuoti.
func mergeMaxState(local, remote shared.GossipState) shared.GossipState {
	local.EnsureMaxMetadata()
	localInitialized := len(local.AggregationData.Max.Versions) > 0
	ensureIncomingMaxMetadata(&remote)
	appliedRemote := false

	for nodeID, remoteVersion := range remote.AggregationData.Max.Versions {
		localVersion, exists := local.AggregationData.Max.Versions[nodeID]
		if exists && compareVersion(remoteVersion, localVersion) <= 0 {
			continue
		}
		local.AggregationData.Max.Versions[nodeID] = remoteVersion
		appliedRemote = true
	}

	if remote.NodeID != "" {
		remoteContributionVersion := normalizeVersion(remote)
		localContributionVersion := local.AggregationData.Max.Versions[remote.NodeID]
		if compareVersion(remoteContributionVersion, localContributionVersion) > 0 {
			local.AggregationData.Max.Versions[remote.NodeID] = remoteContributionVersion
			appliedRemote = true
		}
	}

	if !localInitialized {
		local.Value = remote.Value
		return local
	}
	if appliedRemote {
		local.Value = math.Max(local.Value, remote.Value)
	}
	return local
}

// ensureIncomingMaxMetadata rende compatibili i messaggi legacy senza metadati max.
func ensureIncomingMaxMetadata(state *shared.GossipState) {
	if state.AggregationType != "max" {
		return
	}
	state.EnsureMaxMetadata()
	if state.NodeID == "" {
		return
	}
	version := normalizeVersion(*state)
	knownVersion, ok := state.AggregationData.Max.Versions[state.NodeID]
	if !ok || compareVersion(version, knownVersion) > 0 {
		state.AggregationData.Max.Versions[state.NodeID] = version
	}
}

// averageFromContributions calcola la media aggregando i contributi noti e ignorando count zero.
func averageFromContributions(contributions map[shared.NodeID]shared.AverageContribution) float64 {
	if len(contributions) == 0 {
		return 0
	}
	totalSum := 0.0
	var totalCount uint64
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
	switch local.AggregationType {
	case "sum":
		return sameSumPayload(local, remote)
	case "average":
		return sameAveragePayload(local, remote)
	case "min":
		return sameMinPayload(local, remote)
	case "max":
		return sameMaxPayload(local, remote)
	default:
		return math.Abs(local.Value-remote.Value) < 1e-9
	}
}

func samePayloadSemantically(local, remote shared.GossipState) bool {
	if local.AggregationType != remote.AggregationType {
		return false
	}
	switch local.AggregationType {
	case "average":
		return sameAveragePayloadSemantically(local, remote)
	case "min":
		return sameMinPayloadSemantically(local, remote)
	case "max":
		return sameMaxPayloadSemantically(local, remote)
	default:
		return false
	}
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

func sameAveragePayload(local, remote shared.GossipState) bool {
	local.EnsureAverageMetadata()
	ensureIncomingAverageMetadata(&remote)
	if len(local.AggregationData.Average.Contributions) != len(remote.AggregationData.Average.Contributions) {
		return false
	}
	for nodeID, localValue := range local.AggregationData.Average.Contributions {
		remoteValue, ok := remote.AggregationData.Average.Contributions[nodeID]
		if !ok {
			return false
		}
		if math.Abs(localValue.Sum-remoteValue.Sum) > 1e-9 || localValue.Count != remoteValue.Count {
			return false
		}
		if compareVersion(local.AggregationData.Average.Versions[nodeID], remote.AggregationData.Average.Versions[nodeID]) != 0 {
			return false
		}
	}
	return true
}

func sameAveragePayloadSemantically(local, remote shared.GossipState) bool {
	local.EnsureAverageMetadata()
	ensureIncomingAverageMetadata(&remote)
	if math.Abs(averageFromContributions(local.AggregationData.Average.Contributions)-averageFromContributions(remote.AggregationData.Average.Contributions)) > 1e-9 {
		return false
	}
	return averageMetadataCompatible(local.AggregationData.Average, remote.AggregationData.Average)
}

func sameMinPayload(local, remote shared.GossipState) bool {
	local.EnsureMinMetadata()
	ensureIncomingMinMetadata(&remote)
	if math.Abs(local.Value-remote.Value) > 1e-9 {
		return false
	}
	if len(local.AggregationData.Min.Versions) != len(remote.AggregationData.Min.Versions) {
		return false
	}
	for nodeID, localVersion := range local.AggregationData.Min.Versions {
		if compareVersion(localVersion, remote.AggregationData.Min.Versions[nodeID]) != 0 {
			return false
		}
	}
	return true
}

func sameMinPayloadSemantically(local, remote shared.GossipState) bool {
	local.EnsureMinMetadata()
	ensureIncomingMinMetadata(&remote)
	if math.Abs(local.Value-remote.Value) > 1e-9 {
		return false
	}
	return versionMapsCompatible(local.AggregationData.Min.Versions, remote.AggregationData.Min.Versions)
}

func sameMaxPayload(local, remote shared.GossipState) bool {
	local.EnsureMaxMetadata()
	ensureIncomingMaxMetadata(&remote)
	if math.Abs(local.Value-remote.Value) > 1e-9 {
		return false
	}
	if len(local.AggregationData.Max.Versions) != len(remote.AggregationData.Max.Versions) {
		return false
	}
	for nodeID, localVersion := range local.AggregationData.Max.Versions {
		if compareVersion(localVersion, remote.AggregationData.Max.Versions[nodeID]) != 0 {
			return false
		}
	}
	return true
}

func sameMaxPayloadSemantically(local, remote shared.GossipState) bool {
	local.EnsureMaxMetadata()
	ensureIncomingMaxMetadata(&remote)
	if math.Abs(local.Value-remote.Value) > 1e-9 {
		return false
	}
	return versionMapsCompatible(local.AggregationData.Max.Versions, remote.AggregationData.Max.Versions)
}

func averageMetadataCompatible(local, remote *shared.AverageState) bool {
	for nodeID, localVersion := range local.Versions {
		remoteVersion, ok := remote.Versions[nodeID]
		if !ok || compareVersion(localVersion, remoteVersion) != 0 {
			continue
		}
		localContribution, localContributionOK := local.Contributions[nodeID]
		remoteContribution, remoteContributionOK := remote.Contributions[nodeID]
		if !localContributionOK || !remoteContributionOK {
			return false
		}
		if math.Abs(localContribution.Sum-remoteContribution.Sum) > 1e-9 || localContribution.Count != remoteContribution.Count {
			return false
		}
	}
	for nodeID, remoteVersion := range remote.Versions {
		localVersion, ok := local.Versions[nodeID]
		if !ok || compareVersion(remoteVersion, localVersion) != 0 {
			continue
		}
		localContribution, localContributionOK := local.Contributions[nodeID]
		remoteContribution, remoteContributionOK := remote.Contributions[nodeID]
		if !localContributionOK || !remoteContributionOK {
			return false
		}
		if math.Abs(localContribution.Sum-remoteContribution.Sum) > 1e-9 || localContribution.Count != remoteContribution.Count {
			return false
		}
	}
	return true
}

func versionMapsCompatible(local, remote map[shared.NodeID]shared.StateVersionStamp) bool {
	for nodeID, localVersion := range local {
		remoteVersion, ok := remote[nodeID]
		if !ok {
			continue
		}
		if compareVersion(localVersion, remoteVersion) != 0 {
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
