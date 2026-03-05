package types

import "time"

// NodeID identifica univocamente un nodo nel cluster.
type NodeID string

// MessageID identifica univocamente un messaggio gossip.
type MessageID string

// StateVersion rappresenta la versione monotona dello stato condiviso.
type StateVersion uint64

// MessageEnvelope contiene metadati trasversali del messaggio.
type MessageEnvelope struct {
	MessageID    MessageID `json:"message_id"`
	SenderNodeID NodeID    `json:"sender_node_id"`
	SentAt       time.Time `json:"sent_at"`
}

// GossipState rappresenta lo stato serializzabile condiviso tra package.
type GossipState struct {
	NodeID          NodeID       `json:"node_id"`
	Round           StateVersion `json:"round"`
	AggregationType string       `json:"aggregation_type"`
	Value           float64      `json:"value"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// GossipMessage è il payload gossip con envelope e stato.
type GossipMessage struct {
	Envelope MessageEnvelope   `json:"envelope"`
	State    GossipState       `json:"state"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
