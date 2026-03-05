package gossip

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
)

func TestEngineStartStop(t *testing.T) {
	eng := NewEngine(
		"node-1",
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
