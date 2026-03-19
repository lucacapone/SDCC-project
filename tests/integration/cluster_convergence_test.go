package integration_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"sdcc-project/internal/gossip"
	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
)

const (
	clusterBootstrapStrategy = "harness in-memory promosso"
	m09NodeCount             = 3
	m09Aggregation           = "average"
	m09GossipInterval        = 10 * time.Millisecond
	m09PollInterval          = 20 * time.Millisecond
	m09BootstrapAllowance    = 5 * m09GossipInterval
	m09LocalCIBuffer         = 15 * m09PollInterval
	m09Timeout               = m09BootstrapAllowance + m09LocalCIBuffer
	m09ConvergenceBand       = 0.05
)

// integrationNetwork modella una rete in-memory deterministicamente controllabile per il cluster di test.
type integrationNetwork struct {
	mu         sync.RWMutex
	transports map[string]*integrationTransport
}

// newIntegrationNetwork crea il registro condiviso degli endpoint di test.
func newIntegrationNetwork() *integrationNetwork {
	return &integrationNetwork{transports: make(map[string]*integrationTransport)}
}

// newTransport costruisce un transport associato a un endpoint logico del cluster.
func (n *integrationNetwork) newTransport(address string) *integrationTransport {
	return &integrationTransport{address: address, network: n}
}

// register rende raggiungibile un transport all'interno della rete in-memory.
func (n *integrationNetwork) register(address string, tr *integrationTransport) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.transports[address] = tr
}

// unregister rimuove il transport dalla rete durante lo shutdown del nodo.
func (n *integrationNetwork) unregister(address string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.transports, address)
}

// deliver inoltra il payload al destinatario simulando un canale locale senza rete reale.
func (n *integrationNetwork) deliver(ctx context.Context, to string, payload []byte) error {
	n.mu.RLock()
	destination := n.transports[to]
	n.mu.RUnlock()
	if destination == nil {
		return fmt.Errorf("peer %s non registrato", to)
	}
	return destination.handle(ctx, payload)
}

// integrationTransport implementa il contratto Transport con delivery sincrono e copiatura difensiva del payload.
type integrationTransport struct {
	address string
	network *integrationNetwork

	mu      sync.RWMutex
	handler transport.MessageHandler
	closed  bool
}

// Start registra l'handler del nodo sulla rete in-memory.
func (t *integrationTransport) Start(_ context.Context, handler transport.MessageHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if handler == nil {
		return errors.New("handler nil")
	}
	t.handler = handler
	t.closed = false
	t.network.register(t.address, t)
	return nil
}

// Send recapita sincronicamente una copia del payload al destinatario registrato.
func (t *integrationTransport) Send(ctx context.Context, to string, payload []byte) error {
	t.mu.RLock()
	closed := t.closed
	t.mu.RUnlock()
	if closed {
		return errors.New("transport chiuso")
	}
	copyPayload := append([]byte(nil), payload...)
	return t.network.deliver(ctx, to, copyPayload)
}

// Close deregistra il nodo dalla rete in-memory.
func (t *integrationTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	t.network.unregister(t.address)
	return nil
}

// handle esegue l'handler registrato per simulare la ricezione del messaggio.
func (t *integrationTransport) handle(ctx context.Context, payload []byte) error {
	t.mu.RLock()
	handler := t.handler
	t.mu.RUnlock()
	if handler == nil {
		return errors.New("handler non inizializzato")
	}
	return handler(ctx, payload)
}

// clusterNode raccoglie engine e riferimenti di debug per il test di integrazione.
type clusterNode struct {
	address string
	engine  *gossip.Engine
}

// clusterObservation rappresenta uno snapshot osservabile della convergenza del cluster.
type clusterObservation struct {
	values             map[string]float64
	referenceValue     float64
	maxDelta           float64
	referenceMaxOffset float64
}

// TestClusterConvergence verifica che un cluster a tre nodi converga entro la banda e il timeout ufficiali M09.
func TestClusterConvergence(t *testing.T) {
	initialValues := []float64{10, 30, 50}
	expectedValue := averageOf(initialValues)

	t.Logf("bootstrap cluster automatico con strategia %q", clusterBootstrapStrategy)
	t.Logf("parametri M09: nodi=%d aggregazione=%s gossip_interval=%s poll_interval=%s bootstrap_allowance=%s buffer_locale_ci=%s timeout=%s banda=%0.6f",
		m09NodeCount,
		m09Aggregation,
		m09GossipInterval,
		m09PollInterval,
		m09BootstrapAllowance,
		m09LocalCIBuffer,
		m09Timeout,
		m09ConvergenceBand,
	)

	nodes, cancel := bootstrapAverageCluster(t, initialValues, m09GossipInterval)
	defer cancel()
	defer stopCluster(t, nodes)

	observation, converged := waitForClusterConvergence(nodes, m09Timeout, m09PollInterval, expectedValue, m09ConvergenceBand)
	t.Logf("report finale convergenza:\n%s", formatClusterObservation(observation))

	if !converged {
		t.Fatalf(
			"cluster non convergente entro %s: banda<=%0.6f report=%s",
			m09Timeout,
			m09ConvergenceBand,
			formatClusterObservation(observation),
		)
	}
}

// bootstrapAverageCluster avvia automaticamente un cluster full-mesh con aggregazione average.
func bootstrapAverageCluster(t *testing.T, initialValues []float64, roundEvery time.Duration) ([]*clusterNode, context.CancelFunc) {
	t.Helper()

	network := newIntegrationNetwork()
	ctx, cancel := context.WithCancel(context.Background())
	addresses := make([]string, 0, len(initialValues))
	for index := range initialValues {
		addresses = append(addresses, fmt.Sprintf("node-%d", index+1))
	}

	nodes := make([]*clusterNode, 0, len(initialValues))
	for index, value := range initialValues {
		address := addresses[index]
		engine := gossip.NewEngine(
			address,
			m09Aggregation,
			network.newTransport(address),
			fullMeshMembership(address, addresses),
			slog.Default(),
			roundEvery,
		)
		engine.State.Value = value

		if err := engine.Start(ctx); err != nil {
			cancel()
			stopCluster(t, nodes)
			t.Fatalf("start nodo %s: %v", address, err)
		}

		nodes = append(nodes, &clusterNode{address: address, engine: engine})
	}

	return nodes, cancel
}

// fullMeshMembership costruisce la membership locale iniziale escludendo il nodo corrente.
func fullMeshMembership(self string, addresses []string) *membership.Set {
	set := membership.NewSet()
	now := time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)
	for _, address := range addresses {
		if address == self {
			continue
		}
		set.Join(address, now)
	}
	return set
}

// waitForClusterConvergence effettua polling con deadline esplicita fino a osservare convergenza verificabile.
func waitForClusterConvergence(nodes []*clusterNode, timeout time.Duration, pollEvery time.Duration, expectedValue float64, threshold float64) (clusterObservation, bool) {
	observation := observeCluster(nodes, expectedValue)
	if isClusterConverged(observation, threshold) {
		return observation, true
	}

	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-ticker.C:
			observation = observeCluster(nodes, expectedValue)
			if isClusterConverged(observation, threshold) {
				return observation, true
			}
		case <-timeoutTimer.C:
			return observation, false
		}
	}
}

// observeCluster estrae lo snapshot corrente e calcola le metriche di convergenza del cluster.
func observeCluster(nodes []*clusterNode, expectedValue float64) clusterObservation {
	values := make(map[string]float64, len(nodes))
	for _, node := range nodes {
		values[node.address] = node.engine.State.Value
	}

	return clusterObservation{
		values:             values,
		referenceValue:     expectedValue,
		maxDelta:           observationMaxDelta(values),
		referenceMaxOffset: observationMaxDistance(values, expectedValue),
	}
}

// isClusterConverged rende esplicito il criterio di convergenza: banda massima tra i nodi entro soglia.
func isClusterConverged(observation clusterObservation, threshold float64) bool {
	return observation.maxDelta <= threshold
}

// formatClusterObservation produce un report leggibile dei valori finali per nodo.
func formatClusterObservation(observation clusterObservation) string {
	if len(observation.values) == 0 {
		return "cluster vuoto"
	}

	ordered := []string{"node-1", "node-2", "node-3"}
	parts := make([]string, 0, len(observation.values)+3)
	seen := make(map[string]struct{}, len(observation.values))
	for _, address := range ordered {
		value, ok := observation.values[address]
		if !ok {
			continue
		}
		parts = append(parts, formatNodeObservation(address, value, observation.referenceValue, observation.maxDelta))
		seen[address] = struct{}{}
	}
	for address, value := range observation.values {
		if _, ok := seen[address]; ok {
			continue
		}
		parts = append(parts, formatNodeObservation(address, value, observation.referenceValue, observation.maxDelta))
	}

	parts = append(parts,
		fmt.Sprintf("riferimento_media_iniziale=%0.6f", observation.referenceValue),
		fmt.Sprintf("banda=%0.6f", observation.maxDelta),
		fmt.Sprintf("criterio_successo=banda<=%0.6f", m09ConvergenceBand),
		fmt.Sprintf("offset_max_riferimento=%0.6f", observation.referenceMaxOffset),
	)

	return strings.Join(parts, ", ")
}

// formatNodeObservation rende esplicito il report finale per nodo nel formato M09.
func formatNodeObservation(nodeID string, observedValue float64, expectedValue float64, commonBand float64) string {
	return fmt.Sprintf(
		"node_id=%s observed_value=%0.6f expected_delta=%0.6f common_band=%0.6f",
		nodeID,
		observedValue,
		math.Abs(observedValue-expectedValue),
		commonBand,
	)
}

// observationMaxDelta calcola la massima distanza assoluta tra i valori osservati nel cluster.
func observationMaxDelta(values map[string]float64) float64 {
	if len(values) < 2 {
		return 0
	}

	first := true
	var minValue float64
	var maxValue float64
	for _, value := range values {
		if first {
			minValue = value
			maxValue = value
			first = false
			continue
		}
		minValue = math.Min(minValue, value)
		maxValue = math.Max(maxValue, value)
	}

	return math.Abs(maxValue - minValue)
}

// observationMaxDistance calcola la distanza assoluta massima dal valore atteso comune.
func observationMaxDistance(values map[string]float64, expectedValue float64) float64 {
	maxDistance := 0.0
	for _, value := range values {
		distance := math.Abs(value - expectedValue)
		if distance > maxDistance {
			maxDistance = distance
		}
	}
	return maxDistance
}

// averageOf calcola il valore atteso comune del cluster nel caso average.
func averageOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

// stopCluster arresta tutti gli engine avviati dal test ignorando i nodi nil.
func stopCluster(t *testing.T, nodes []*clusterNode) {
	t.Helper()
	for _, node := range nodes {
		if node == nil || node.engine == nil {
			continue
		}
		if err := node.engine.Stop(); err != nil {
			t.Fatalf("stop nodo %s: %v", node.address, err)
		}
	}
}
