package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sdcc-project/internal/config"
	"sdcc-project/internal/gossip"
	"sdcc-project/internal/membership"
	"sdcc-project/internal/observability"
	"sdcc-project/internal/transport"
)

func main() {
	configPath := flag.String("config", "", "percorso file configurazione")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		// Bootstrap minimo: fallback su default se parser non ancora implementato.
		cfg = config.Default()
	}
	if err := config.Validate(cfg); err != nil {
		panic(err)
	}

	logger := observability.NewLogger(cfg.LogLevel, nil)
	mset := membership.NewSet()
	for _, p := range cfg.Peers {
		mset.Join(p, time.Now().UTC())
	}

	eng := gossip.NewEngine(
		cfg.NodeID,
		transport.NoopTransport{},
		mset,
		logger,
		time.Duration(cfg.RoundEveryMS)*time.Millisecond,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		panic(err)
	}
	<-ctx.Done()
	_ = eng.Stop()
}
