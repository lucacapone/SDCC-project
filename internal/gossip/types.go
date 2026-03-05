package gossip

import "time"

// Message è il payload minimo scambiato tra nodi gossip.
type Message struct {
	NodeID          string            `json:"node_id"`
	Round           uint64            `json:"round"`
	AggregationType string            `json:"aggregation_type"`
	Value           float64           `json:"value"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	SentAt          time.Time         `json:"sent_at"`
}

// State rappresenta lo stato locale gossip di un nodo.
type State struct {
	NodeID          string
	Round           uint64
	AggregationType string
	Value           float64
	UpdatedAt       time.Time
}
