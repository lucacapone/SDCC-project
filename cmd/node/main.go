package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"sdcc-project/internal/aggregation"
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
		log.Fatalf("errore caricamento configurazione: %v", err)
	}

	selfAdvertiseAddr := cfg.AdvertiseEndpoint()
	logger := observability.NewLogger(cfg.LogLevel, nil)
	mset := membership.NewSet()
	bootstrapRes := membership.Bootstrap(
		context.Background(),
		mset,
		membership.JoinRequest{NodeID: cfg.NodeID, Addr: selfAdvertiseAddr},
		cfg.JoinEndpoint,
		cfg.DiscoveryPeers(),
		membership.NoopJoinClient{},
		time.Now().UTC(),
	)
	logger.Info("gossip bootstrap completato",
		"event", "node_bootstrap",
		"node_id", cfg.NodeID,
		"round", 0,
		"peers", bootstrapRes.KnownPeers,
		"estimate", 0.0,
		"join_endpoint", bootstrapRes.JoinEndpoint,
		"used_join_endpoint", bootstrapRes.UsedJoinEndpoint,
		"fallback_used", bootstrapRes.FallbackUsed,
		"advertise_addr", selfAdvertiseAddr,
	)

	listenAddress := net.JoinHostPort(cfg.BindAddress, strconv.Itoa(cfg.NodePort))
	var gossipTransport transport.Transport
	udpTransport, err := transport.NewUDPTransport(listenAddress)
	if err != nil {
		logger.Warn("avvio transport gossip con fallback noop",
			"event", "transport_start",
			"node_id", cfg.NodeID,
			"round", 0,
			"peers", bootstrapRes.KnownPeers,
			"estimate", 0.0,
			"transport", "noop",
			"listen_address", fmt.Sprintf("udp://%s", listenAddress),
			"advertise_address", fmt.Sprintf("udp://%s", selfAdvertiseAddr),
			"error", err,
		)
		gossipTransport = transport.NoopTransport{}
	} else {
		gossipTransport = udpTransport
		logger.Info("transport gossip avviato",
			"event", "transport_start",
			"node_id", cfg.NodeID,
			"round", 0,
			"peers", bootstrapRes.KnownPeers,
			"estimate", 0.0,
			"transport", "udp",
			"listen_address", fmt.Sprintf("udp://%s", listenAddress),
			"advertise_address", fmt.Sprintf("udp://%s", selfAdvertiseAddr),
		)
	}

	aggAlgo, err := aggregation.Factory(cfg.Aggregation)
	if err != nil {
		log.Fatalf("errore inizializzazione aggregazione: %v", err)
	}

	eng := gossip.NewEngine(
		cfg.NodeID,
		aggAlgo.Type(),
		gossipTransport,
		mset,
		logger,
		time.Duration(cfg.GossipIntervalMS)*time.Millisecond,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		panic(err)
	}
	<-ctx.Done()

	// Registra nei log lo snapshot finale per rendere osservabile il risultato del nodo
	// durante il teardown orchestrato dagli script Docker Compose.
	logger.Info("shutdown nodo completato",
		"event", "shutdown",
		"node_id", cfg.NodeID,
		"round", eng.State.Round,
		"peers", len(mset.Snapshot()),
		"estimate", eng.State.Value,
		"aggregation", eng.State.AggregationType,
		"last_message_id", eng.State.LastMessageID,
	)

	_ = eng.Stop()
}
