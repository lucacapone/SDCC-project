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
	"strings"
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
	collector := observability.NewCollector(time.Now().UTC())
	collector.SetHealthMessage("alive")
	collector.SetNodeState(observability.NodeStateStartup)
	metricsHandler := observability.NewMetricsHandler(collector)
	metricsAddr := observabilityAddress()
	metricsServer := observability.NewServer(metricsAddr, metricsHandler.Handler())
	go func() {
		if err := metricsServer.Start(); err != nil {
			logger.Warn("server observability terminato con errore",
				"event", "observability_http",
				"node_id", cfg.NodeID,
				"round", 0,
				"peers", 0,
				"estimate", 0.0,
				"listen_address", metricsAddr,
				"error", err,
			)
		}
	}()
	mset := membership.NewSetWithConfig(cfg.MembershipConfig())
	// Registra subito l'identità locale canonica in membership prima del bootstrap e
	// prima dell'avvio dell'engine gossip, così i filtri self possono riconoscere sia
	// node_id logico sia advertise_addr anche in fase di startup.
	mset.SetSelfNodeID(cfg.NodeID)
	mset.TouchOrUpsertCanonical(cfg.NodeID, selfAdvertiseAddr, time.Now().UTC())
	joinClient := selectJoinClient(cfg)
	bootstrapRes := membership.Bootstrap(
		context.Background(),
		mset,
		membership.JoinRequest{NodeID: cfg.NodeID, Addr: selfAdvertiseAddr},
		cfg.JoinEndpoint,
		cfg.DiscoveryPeers(),
		joinClient,
		time.Now().UTC(),
	)
	collector.AdvanceNodeState(observability.NodeStateBootstrapCompleted)
	collector.SetKnownPeers(bootstrapRes.KnownPeers)
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
		collector.AdvanceNodeState(observability.NodeStateTransportInitialized)
	} else {
		gossipTransport = udpTransport
		collector.AdvanceNodeState(observability.NodeStateTransportInitialized)
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
		collector,
		time.Duration(cfg.GossipIntervalMS)*time.Millisecond,
	)
	// Conserviamo il valore locale originario in uno stato runtime dedicato per evitare
	// che l'algoritmo average sovrascriva il contributo del nodo con la media corrente.
	eng.State.LocalValue = cfg.InitialValue
	eng.State.Value = cfg.InitialValue

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		panic(err)
	}
	collector.AdvanceNodeState(observability.NodeStateEngineStarted)
	collector.SetKnownPeers(len(mset.Snapshot()))
	collector.SetCurrentEstimate(eng.State.Value)
	<-ctx.Done()

	// Registra nei log lo snapshot finale per rendere osservabile il risultato del nodo
	// durante il teardown orchestrato dagli script Docker Compose.
	collector.SetCurrentEstimate(eng.State.Value)
	collector.SetKnownPeers(len(mset.Snapshot()))
	collector.SetNodeState(observability.NodeStateShutdown)
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
	_ = metricsServer.Shutdown(5 * time.Second)
}

// selectJoinClient usa il client HTTP reale quando join_endpoint è configurato,
// mantenendo il fallback storico sul client noop negli altri casi.
func selectJoinClient(cfg config.Config) membership.JoinClient {
	if strings.TrimSpace(cfg.JoinEndpoint) == "" {
		return membership.NoopJoinClient{}
	}
	return membership.NewHTTPJoinClient(3 * time.Second)
}

// observabilityAddress restituisce l'indirizzo del server HTTP di observability.
//
// Se OBSERVABILITY_ADDR non è valorizzato, usa :8080 per mantenere il wiring
// piccolo e compatibile con Compose/debug locale senza introdurre nuova config.
func observabilityAddress() string {
	addr := strings.TrimSpace(os.Getenv("OBSERVABILITY_ADDR"))
	if addr == "" {
		return ":8080"
	}
	return addr
}
