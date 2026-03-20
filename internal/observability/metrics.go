package observability

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// mergeResultUnknown raccoglie eventuali esiti non riconosciuti senza aumentare la cardinalità.
	mergeResultUnknown = "unknown"
)

var allowedMergeResults = map[string]struct{}{
	"applied":          {},
	"skipped":          {},
	"conflict":         {},
	mergeResultUnknown: {},
}

// Collector raccoglie metriche aggregate del nodo con label a bassa cardinalità.
type Collector struct {
	mu sync.RWMutex

	startedAt time.Time
	ready     bool

	totalRounds   uint64
	remoteMerges  map[string]uint64
	currentPeers  int
	currentValue  float64
	healthMessage string
}

// Snapshot rappresenta una vista coerente e immutabile delle metriche correnti.
type Snapshot struct {
	StartedAt       time.Time
	Ready           bool
	TotalRounds     uint64
	RemoteMerges    map[string]uint64
	KnownPeers      int
	CurrentEstimate float64
	Uptime          time.Duration
	HealthMessage   string
}

// NewCollector costruisce un collector inizializzato con uptime consistente e stato not ready.
func NewCollector(now time.Time) *Collector {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	collector := &Collector{
		startedAt:     now,
		remoteMerges:  make(map[string]uint64, len(allowedMergeResults)),
		healthMessage: "ok",
	}
	for result := range allowedMergeResults {
		collector.remoteMerges[result] = 0
	}
	return collector
}

// IncTotalRounds incrementa il contatore dei round gossip completati localmente.
func (c *Collector) IncTotalRounds() {
	c.AddRounds(1)
}

// AddRounds incrementa il numero totale di round gossip di un delta non negativo.
func (c *Collector) AddRounds(delta uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.totalRounds += delta
}

// IncRemoteMergeOutcome incrementa il contatore di merge remoti per esito.
func (c *Collector) IncRemoteMergeOutcome(result string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	normalized := normalizeMergeResult(result)
	c.remoteMerges[normalized]++
}

// SetKnownPeers aggiorna il numero corrente di peer noti.
func (c *Collector) SetKnownPeers(peers int) {
	if peers < 0 {
		peers = 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentPeers = peers
}

// SetCurrentEstimate aggiorna la stima corrente locale del nodo.
func (c *Collector) SetCurrentEstimate(value float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentValue = value
}

// SetReady aggiorna lo stato di readiness del nodo/servizio.
func (c *Collector) SetReady(ready bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ready = ready
}

// SetHealthMessage aggiorna il messaggio sintetico dell'health check.
func (c *Collector) SetHealthMessage(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "ok"
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.healthMessage = message
}

// Snapshot restituisce una copia coerente dello stato del collector.
func (c *Collector) Snapshot(now time.Time) Snapshot {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	remoteMerges := make(map[string]uint64, len(c.remoteMerges))
	for result, count := range c.remoteMerges {
		remoteMerges[result] = count
	}

	return Snapshot{
		StartedAt:       c.startedAt,
		Ready:           c.ready,
		TotalRounds:     c.totalRounds,
		RemoteMerges:    remoteMerges,
		KnownPeers:      c.currentPeers,
		CurrentEstimate: c.currentValue,
		Uptime:          now.Sub(c.startedAt),
		HealthMessage:   c.healthMessage,
	}
}

// MetricsHandler espone gli endpoint HTTP di health, readiness e metriche aggregate.
type MetricsHandler struct {
	collector *Collector
	now       func() time.Time
}

// NewMetricsHandler costruisce un handler HTTP minimale per observability locale.
func NewMetricsHandler(collector *Collector) *MetricsHandler {
	if collector == nil {
		collector = NewCollector(time.Now().UTC())
	}
	return &MetricsHandler{collector: collector, now: func() time.Time { return time.Now().UTC() }}
}

// Handler restituisce l'handler HTTP con endpoint distinti /health, /ready e /metrics.
func (h *MetricsHandler) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/ready", h.handleReady)
	mux.HandleFunc("/metrics", h.handleMetrics)
	return mux
}

// Collector restituisce il collector sottostante per eventuali aggiornamenti runtime.
func (h *MetricsHandler) Collector() *Collector {
	return h.collector
}

// Server rappresenta un piccolo server HTTP dedicato agli endpoint di observability.
type Server struct {
	httpServer *http.Server
}

// NewServer costruisce un server HTTP minimale per esporre gli endpoint osservabili.
func NewServer(addr string, handler http.Handler) *Server {
	if strings.TrimSpace(addr) == "" {
		addr = ":8080"
	}
	if handler == nil {
		handler = http.NewServeMux()
	}
	return &Server{httpServer: &http.Server{Addr: addr, Handler: handler}}
}

// Start avvia il server HTTP e ritorna nil su shutdown atteso.
func (s *Server) Start() error {
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown arresta il server HTTP rispettando il context del chiamante.
func (s *Server) Shutdown(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// handleHealth espone lo stato di liveness del processo.
func (h *MetricsHandler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	snapshot := h.collector.Snapshot(h.now())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "{\"status\":\"ok\",\"message\":%q}", snapshot.HealthMessage)
}

// handleReady espone lo stato di readiness del nodo/servizio.
func (h *MetricsHandler) handleReady(w http.ResponseWriter, _ *http.Request) {
	snapshot := h.collector.Snapshot(h.now())
	w.Header().Set("Content-Type", "application/json")
	if !snapshot.Ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, "{\"status\":\"not_ready\"}")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "{\"status\":\"ready\"}")
}

// handleMetrics espone metriche testuali in formato line-based stabile e facile da testare.
func (h *MetricsHandler) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	snapshot := h.collector.Snapshot(h.now())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(formatMetrics(snapshot)))
}

// formatMetrics serializza le metriche con naming stabile e label a bassa cardinalità.
func formatMetrics(snapshot Snapshot) string {
	var builder strings.Builder

	builder.WriteString("# HELP sdcc_node_rounds_total Round gossip completati dal nodo.\n")
	builder.WriteString("# TYPE sdcc_node_rounds_total counter\n")
	builder.WriteString("sdcc_node_rounds_total ")
	builder.WriteString(strconv.FormatUint(snapshot.TotalRounds, 10))
	builder.WriteString("\n")

	builder.WriteString("# HELP sdcc_node_remote_merges_total Merge remoti raggruppati per esito.\n")
	builder.WriteString("# TYPE sdcc_node_remote_merges_total counter\n")
	results := make([]string, 0, len(snapshot.RemoteMerges))
	for result := range snapshot.RemoteMerges {
		results = append(results, result)
	}
	sort.Strings(results)
	for _, result := range results {
		builder.WriteString("sdcc_node_remote_merges_total{result=\"")
		builder.WriteString(result)
		builder.WriteString("\"} ")
		builder.WriteString(strconv.FormatUint(snapshot.RemoteMerges[result], 10))
		builder.WriteString("\n")
	}

	builder.WriteString("# HELP sdcc_node_known_peers Peer noti correnti nel nodo.\n")
	builder.WriteString("# TYPE sdcc_node_known_peers gauge\n")
	builder.WriteString("sdcc_node_known_peers ")
	builder.WriteString(strconv.Itoa(snapshot.KnownPeers))
	builder.WriteString("\n")

	builder.WriteString("# HELP sdcc_node_estimate Stima corrente locale del nodo.\n")
	builder.WriteString("# TYPE sdcc_node_estimate gauge\n")
	builder.WriteString("sdcc_node_estimate ")
	builder.WriteString(strconv.FormatFloat(snapshot.CurrentEstimate, 'f', -1, 64))
	builder.WriteString("\n")

	builder.WriteString("# HELP sdcc_node_uptime_seconds Uptime del processo in secondi.\n")
	builder.WriteString("# TYPE sdcc_node_uptime_seconds gauge\n")
	builder.WriteString("sdcc_node_uptime_seconds ")
	builder.WriteString(strconv.FormatFloat(snapshot.Uptime.Seconds(), 'f', 3, 64))
	builder.WriteString("\n")

	builder.WriteString("# HELP sdcc_node_ready Stato di readiness del nodo (1 ready, 0 not ready).\n")
	builder.WriteString("# TYPE sdcc_node_ready gauge\n")
	builder.WriteString("sdcc_node_ready ")
	if snapshot.Ready {
		builder.WriteString("1\n")
	} else {
		builder.WriteString("0\n")
	}

	return builder.String()
}

// normalizeMergeResult riduce gli esiti ignoti a un bucket stabile per preservare bassa cardinalità.
func normalizeMergeResult(result string) string {
	result = strings.ToLower(strings.TrimSpace(result))
	if _, ok := allowedMergeResults[result]; ok {
		return result
	}
	return mergeResultUnknown
}
