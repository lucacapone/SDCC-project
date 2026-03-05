package gossip

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
	shared "sdcc-project/internal/types"
)

var currentMessageVersion = shared.MessageVersion{Major: 1, Minor: 0}

// Engine coordina il ciclo gossip locale.
type Engine struct {
	NodeID      shared.NodeID
	State       shared.GossipState
	Membership  *membership.Set
	Transport   transport.Transport
	Logger      *slog.Logger
	RoundTicker *time.Ticker
	mu          sync.Mutex
}

// NewEngine costruisce un engine con dipendenze minime.
func NewEngine(nodeID, aggregationType string, t transport.Transport, m *membership.Set, logger *slog.Logger, roundEvery time.Duration) *Engine {
	if roundEvery <= 0 {
		roundEvery = time.Second
	}
	return &Engine{
		NodeID: shared.NodeID(nodeID),
		State: shared.GossipState{
			NodeID:          shared.NodeID(nodeID),
			AggregationType: aggregationType,
			UpdatedAt:       time.Now().UTC(),
		},
		Membership:  m,
		Transport:   t,
		Logger:      logger,
		RoundTicker: time.NewTicker(roundEvery),
	}
}

// Start avvia il transport e il loop gossip.
// TODO(tecnico): introdurre fanout random, retry e gestione backpressure.
func (e *Engine) Start(ctx context.Context) error {
	if e.Transport == nil {
		return fmt.Errorf("transport nil")
	}
	if e.Membership == nil {
		return fmt.Errorf("membership nil")
	}

	err := e.Transport.Start(ctx, func(_ context.Context, raw []byte) error {
		var msg shared.GossipMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			return err
		}
		normalizeIncomingMessage(&msg)
		e.mu.Lock()
		merge := applyRemote(e.State, msg)
		e.State = merge.State
		e.mu.Unlock()
		if e.Logger != nil {
			e.Logger.Debug("merge remoto", "status", merge.Status, "reason", merge.Reason, "from", msg.OriginNode, "message_id", msg.MessageID)
		}
		return nil
	})
	if err != nil {
		return err
	}

	go e.loop(ctx)
	return nil
}

func (e *Engine) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.RoundTicker.C:
			e.round(ctx)
		}
	}
}

func (e *Engine) round(ctx context.Context) {
	peers := e.Membership.Snapshot()
	sentAt := time.Now().UTC()

	e.mu.Lock()
	nextRound := e.State.Round + 1
	nextVersion := e.State.VersionCounter + 1
	e.State.Round = nextRound
	e.State.VersionCounter = nextVersion
	e.State.UpdatedAt = sentAt

	stateSnapshot := sanitizedStateForMessage(e.State)
	stateVersion := normalizeVersion(stateSnapshot)
	messageID := shared.MessageID(fmt.Sprintf("%s-%d-%d", e.NodeID, nextVersion, sentAt.UnixNano()))
	e.State.LastMessageID = messageID
	e.State.LastSenderNodeID = e.NodeID
	msg := shared.GossipMessage{
		MessageID:    messageID,
		OriginNode:   e.NodeID,
		SentAt:       sentAt,
		Version:      currentMessageVersion,
		StateVersion: stateVersion,
		State:        stateSnapshot,
	}
	e.mu.Unlock()

	raw, _ := json.Marshal(msg)
	for _, p := range peers {
		_ = e.Transport.Send(ctx, p.Addr, raw)
	}

	if e.Logger != nil {
		e.Logger.Debug("gossip round eseguito", "peers", len(peers), "round", msg.State.Round)
	}
}

func sanitizedStateForMessage(state shared.GossipState) shared.GossipState {
	state.SeenMessageIDs = nil
	state.LastSeenVersionByNode = nil
	return state
}

func normalizeIncomingMessage(msg *shared.GossipMessage) {
	if msg.OriginNode == "" {
		msg.OriginNode = msg.State.NodeID
	}
	if msg.SentAt.IsZero() {
		msg.SentAt = msg.State.UpdatedAt
	}
	if msg.MessageID == "" {
		msg.MessageID = shared.MessageID(fmt.Sprintf("legacy-%s-%d", msg.OriginNode, msg.SentAt.UnixNano()))
	}
	if msg.Version == (shared.MessageVersion{}) {
		msg.Version = currentMessageVersion
	}
	if msg.StateVersion == (shared.StateVersionStamp{}) {
		msg.StateVersion = normalizeVersion(msg.State)
	}
}

// Stop ferma ticker e transport.
func (e *Engine) Stop() error {
	if e.RoundTicker != nil {
		e.RoundTicker.Stop()
	}
	if e.Transport != nil {
		return e.Transport.Close()
	}
	return nil
}
