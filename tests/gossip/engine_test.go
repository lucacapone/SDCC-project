package gossip

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
	shared "sdcc-project/internal/types"
)

func TestEngineStartStop(t *testing.T) {
	eng := NewEngine(
		"node-1",
		"sum",
		transport.NoopTransport{},
		membership.NewSet(),
		slog.Default(),
		10*time.Millisecond,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start errore: %v", err)
	}
	if err := eng.Stop(); err != nil {
		t.Fatalf("stop errore: %v", err)
	}
}

type captureTransport struct {
	sent [][]byte
}

func (c *captureTransport) Start(context.Context, transport.MessageHandler) error { return nil }

func (c *captureTransport) Send(_ context.Context, _ string, payload []byte) error {
	c.sent = append(c.sent, append([]byte(nil), payload...))
	return nil
}

func (c *captureTransport) Close() error { return nil }

func TestRoundMessageAndStateVersionAlignment(t *testing.T) {
	tr := &captureTransport{}
	m := membership.NewSet()
	m.Join("node-2", time.Now().UTC())

	eng := NewEngine("node-1", "average", tr, m, slog.Default(), time.Second)
	eng.State.VersionCounter = 2
	eng.State.Round = 2

	eng.RoundOnce(context.Background())

	if len(tr.sent) != 1 {
		t.Fatalf("messaggi inviati inattesi: got=%d want=1", len(tr.sent))
	}

	var msg shared.GossipMessage
	if err := json.Unmarshal(tr.sent[0], &msg); err != nil {
		t.Fatalf("unmarshal messaggio: %v", err)
	}

	if msg.StateVersion != normalizeVersion(msg.State) {
		t.Fatalf("state_version non allineata allo stato serializzato: got=%+v state=%+v", msg.StateVersion, normalizeVersion(msg.State))
	}
	if msg.StateVersion.Counter != 3 {
		t.Fatalf("counter messaggio inatteso: got=%d want=3", msg.StateVersion.Counter)
	}
	if msg.State.Round != 3 {
		t.Fatalf("round messaggio inatteso: got=%d want=3", msg.State.Round)
	}
	if eng.State.VersionCounter != msg.StateVersion.Counter {
		t.Fatalf("versione locale non allineata al messaggio: local=%d msg=%d", eng.State.VersionCounter, msg.StateVersion.Counter)
	}
	if eng.State.Round != msg.State.Round {
		t.Fatalf("round locale non allineato al messaggio: local=%d msg=%d", eng.State.Round, msg.State.Round)
	}
}
