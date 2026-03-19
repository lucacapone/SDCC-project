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
	logger.Info("bootstrap membership completato",
		"used_join_endpoint", bootstrapRes.UsedJoinEndpoint,
		"join_endpoint", bootstrapRes.JoinEndpoint,
		"fallback_used", bootstrapRes.FallbackUsed,
		"known_peers", bootstrapRes.KnownPeers,
		"self_node_id", cfg.NodeID,
		"self_addr", selfAdvertiseAddr,
	)

	listenAddress := net.JoinHostPort(cfg.BindAddress, strconv.Itoa(cfg.NodePort))
	var gossipTransport transport.Transport
	udpTransport, err := transport.NewUDPTransport(listenAddress)
	if err != nil {
		logger.Warn("inizializzazione UDP transport fallita, fallback a NoopTransport", "listen_address", listenAddress, "error", err)
		gossipTransport = transport.NoopTransport{}
	} else {
		gossipTransport = udpTransport
		logger.Info("transport inizializzato", "type", "udp", "listen_address", fmt.Sprintf("udp://%s", listenAddress), "advertise_address", fmt.Sprintf("udp://%s", selfAdvertiseAddr))
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
		"node_id", cfg.NodeID,
		"aggregation", eng.State.AggregationType,
		"final_value", eng.State.Value,
		"final_round", eng.State.Round,
		"last_message_id", eng.State.LastMessageID,
	)

	_ = eng.Stop()
}
