package gossip

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
)

// Engine coordina il ciclo gossip locale.
type Engine struct {
	NodeID      string
	State       State
	Membership  *membership.Set
	Transport   transport.Transport
	Logger      *slog.Logger
	RoundTicker *time.Ticker
}

// NewEngine costruisce un engine con dipendenze minime.
func NewEngine(nodeID string, t transport.Transport, m *membership.Set, logger *slog.Logger, roundEvery time.Duration) *Engine {
	if roundEvery <= 0 {
		roundEvery = time.Second
	}
	return &Engine{
		NodeID:      nodeID,
		State:       State{NodeID: nodeID, AggregationType: "sum", UpdatedAt: time.Now().UTC()},
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
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			return err
		}
		e.State = e.State.ApplyRemote(msg)
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
	msg := Message{
		NodeID:          e.NodeID,
		Round:           e.State.Round,
		AggregationType: e.State.AggregationType,
		Value:           e.State.Value,
		SentAt:          time.Now().UTC(),
	}
	raw, _ := json.Marshal(msg)

	for _, p := range peers {
		_ = e.Transport.Send(ctx, p.Address, raw)
	}

	if e.Logger != nil {
		e.Logger.Debug("gossip round eseguito", "peers", len(peers), "round", e.State.Round)
	}
	e.State.Round++
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
