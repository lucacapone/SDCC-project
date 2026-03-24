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
	NewEngine                                = internalgossip.NewEngine
	MarkPeerAliveForTest                     = internalgossip.MarkPeerAliveForTest
	SerializeMembershipDigestForTest         = internalgossip.SerializeMembershipDigestForTest
	SerializeMembershipDigestWithSelfForTest = internalgossip.SerializeMembershipDigestWithSelfForTest
	currentMessageVersion                    = internalgossip.CurrentMessageVersion()
)

func applyRemote(local shared.GossipState, msg shared.GossipMessage) MergeResult {
	return internalgossip.ApplyRemote(local, msg)
}

func mergeMembership(set *membership.Set, remote []shared.MembershipEntry) {
	internalgossip.MergeMembership(set, remote)
}

func mergeMembershipWithSelf(set *membership.Set, selfNodeID string, remote []shared.MembershipEntry, selfAliases ...string) {
	internalgossip.MergeMembershipWithSelf(set, selfNodeID, remote, selfAliases...)
}

func normalizeVersion(state shared.GossipState) shared.StateVersionStamp {
	return internalgossip.NormalizeStateVersion(state)
}
