package gossip

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
	shared "sdcc-project/internal/types"
)

// Engine coordina il ciclo gossip locale.
type Engine struct {
	NodeID      shared.NodeID
	State       shared.GossipState
	Membership  *membership.Set
	Transport   transport.Transport
	Logger      *slog.Logger
	RoundTicker *time.Ticker
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
		merge := applyRemote(e.State, msg)
		e.State = merge.State
		if e.Logger != nil {
			e.Logger.Debug("merge remoto", "status", merge.Status, "reason", merge.Reason, "from", msg.Envelope.SenderNodeID, "message_id", msg.Envelope.MessageID)
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
	msg := shared.GossipMessage{
		Envelope: shared.MessageEnvelope{
			MessageID:    shared.MessageID(fmt.Sprintf("%s-%d", e.NodeID, e.State.Round)),
			SenderNodeID: e.NodeID,
			SentAt:       time.Now().UTC(),
		},
		State: e.State,
	}
	raw, _ := json.Marshal(msg)

	for _, p := range peers {
		_ = e.Transport.Send(ctx, p.Address, raw)
	}

	if e.Logger != nil {
		e.Logger.Debug("gossip round eseguito", "peers", len(peers), "round", e.State.Round)
	}
	e.State.Round++
	e.State.VersionCounter++
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
