package observability

import internalobservability "sdcc-project/internal/observability"

var (
	// NewLogger re-esporta il costruttore del logger per i test esterni del package logico observability.
	NewLogger = internalobservability.NewLogger
	// NewCollector re-esporta il collector per i test esterni collocati sotto tests/observability.
	NewCollector = internalobservability.NewCollector
	// NewMetricsHandler re-esporta l'handler HTTP osservabile per i test esterni collocati sotto tests/observability.
	NewMetricsHandler = internalobservability.NewMetricsHandler
)

// NodeState riallinea il tipo lifecycle osservabile dal package interno al package di test esterno.
type NodeState = internalobservability.NodeState

const (
	// NodeStateStartup rappresenta l'avvio iniziale del processo osservato dal collector.
	NodeStateStartup = internalobservability.NodeStateStartup
	// NodeStateBootstrapCompleted indica il completamento del bootstrap membership.
	NodeStateBootstrapCompleted = internalobservability.NodeStateBootstrapCompleted
	// NodeStateTransportInitialized indica l'inizializzazione del transport runtime.
	NodeStateTransportInitialized = internalobservability.NodeStateTransportInitialized
	// NodeStateEngineStarted indica che l'engine gossip è pronto.
	NodeStateEngineStarted = internalobservability.NodeStateEngineStarted
	// NodeStateShutdown indica lo shutdown del nodo osservato.
	NodeStateShutdown = internalobservability.NodeStateShutdown
)
