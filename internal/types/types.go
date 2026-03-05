package types

import "time"

// NodeID identifica univocamente un nodo nel cluster.
type NodeID string

// MessageID identifica univocamente un messaggio gossip.
type MessageID string

// StateVersion rappresenta la versione monotona dello stato condiviso.
type StateVersion uint64

// StateVersionStamp rappresenta una versione composta epoch+counter.
type StateVersionStamp struct {
	Epoch   uint64       `json:"epoch"`
	Counter StateVersion `json:"counter"`
}

// MessageVersion descrive in modo esplicito la versione del contratto messaggio.
type MessageVersion struct {
	Major uint16 `json:"major"`
	Minor uint16 `json:"minor"`
}

// GossipState rappresenta lo stato serializzabile condiviso tra package.
type GossipState struct {
	NodeID                NodeID                       `json:"node_id"`
	Round                 StateVersion                 `json:"round"`
	VersionEpoch          uint64                       `json:"version_epoch"`
	VersionCounter        StateVersion                 `json:"version_counter"`
	AggregationType       string                       `json:"aggregation_type"`
	Value                 float64                      `json:"value"`
	UpdatedAt             time.Time                    `json:"updated_at"`
	LastMessageID         MessageID                    `json:"last_message_id,omitempty"`
	LastSenderNodeID      NodeID                       `json:"last_sender_node_id,omitempty"`
	SeenMessageIDs        map[MessageID]struct{}       `json:"-"`
	LastSeenVersionByNode map[NodeID]StateVersionStamp `json:"-"`
}

// GossipMessage è il payload gossip con envelope e stato.
type GossipMessage struct {
	MessageID    MessageID         `json:"message_id"`
	OriginNode   NodeID            `json:"origin_node"`
	SentAt       time.Time         `json:"sent_at"`
	Version      MessageVersion    `json:"version"`
	StateVersion StateVersionStamp `json:"state_version"`
	State        GossipState       `json:"state"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// EnsureMergeMetadata inizializza i campi locali non serializzati necessari al merge.
func (s *GossipState) EnsureMergeMetadata() {
	if s.SeenMessageIDs == nil {
		s.SeenMessageIDs = make(map[MessageID]struct{})
	}
	if s.LastSeenVersionByNode == nil {
		s.LastSeenVersionByNode = make(map[NodeID]StateVersionStamp)
	}
}
