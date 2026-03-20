package gossip

import (
	internalgossip "sdcc-project/internal/gossip"
	"sdcc-project/internal/membership"
	shared "sdcc-project/internal/types"
)

type (
	Engine      = internalgossip.Engine
	MergeStatus = internalgossip.MergeStatus
	MergeResult = internalgossip.MergeResult
)

const (
	MergeApplied  = internalgossip.MergeApplied
	MergeSkipped  = internalgossip.MergeSkipped
	MergeConflict = internalgossip.MergeConflict
)

var (
	NewEngine             = internalgossip.NewEngine
	currentMessageVersion = internalgossip.CurrentMessageVersion()
)

func applyRemote(local shared.GossipState, msg shared.GossipMessage) MergeResult {
	return internalgossip.ApplyRemote(local, msg)
}

func mergeMembership(set *membership.Set, remote []shared.MembershipEntry) {
	internalgossip.MergeMembership(set, remote)
}

func normalizeVersion(state shared.GossipState) shared.StateVersionStamp {
	return internalgossip.NormalizeStateVersion(state)
}
