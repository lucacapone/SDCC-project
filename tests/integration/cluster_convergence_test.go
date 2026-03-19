package integration_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"

	"sdcc-project/internal/gossip"
	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
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

// TestClusterConvergence verifica che un cluster a tre nodi converga entro la banda e il timeout ufficiali M09.
func TestClusterConvergence(t *testing.T) {
	const (
		roundEvery          = 10 * time.Millisecond
		convergenceDeadline = 2 * time.Second
		maxDelta            = 0.05
	)

	network := newIntegrationNetwork()
	nodes, cancel := startAverageCluster(t, network, []float64{10, 30, 50}, roundEvery)
	defer cancel()
	defer stopCluster(t, nodes)

	if ok, maxObservedDelta, snapshots := waitClusterConvergence(nodes, convergenceDeadline, maxDelta); !ok {
		t.Fatalf("cluster non convergente entro %s: delta_max=%0.6f valori=%v", convergenceDeadline, maxObservedDelta, snapshots)
	}
}

// startAverageCluster avvia un cluster full-mesh con aggregazione average e valori iniziali deterministici.
func startAverageCluster(t *testing.T, network *integrationNetwork, initialValues []float64, roundEvery time.Duration) ([]*clusterNode, context.CancelFunc) {
	t.Helper()

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
			"average",
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

// waitClusterConvergence campiona il cluster fino a osservare una differenza massima sotto soglia.
func waitClusterConvergence(nodes []*clusterNode, timeout time.Duration, threshold float64) (bool, float64, []float64) {
	deadline := time.Now().Add(timeout)
	bestDelta := math.MaxFloat64
	bestSnapshot := snapshotValues(nodes)

	for time.Now().Before(deadline) {
		currentSnapshot := snapshotValues(nodes)
		currentDelta := maxDelta(currentSnapshot)
		if currentDelta < bestDelta {
			bestDelta = currentDelta
			bestSnapshot = currentSnapshot
		}
		if currentDelta <= threshold {
			return true, currentDelta, currentSnapshot
		}
		time.Sleep(10 * time.Millisecond)
	}

	return false, bestDelta, bestSnapshot
}

// snapshotValues estrae il valore aggregato corrente di tutti i nodi del cluster.
func snapshotValues(nodes []*clusterNode) []float64 {
	values := make([]float64, 0, len(nodes))
	for _, node := range nodes {
		values = append(values, node.engine.State.Value)
	}
	return values
}

// maxDelta calcola la massima distanza assoluta tra i valori osservati nel cluster.
func maxDelta(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	minValue := values[0]
	maxValue := values[0]
	for _, value := range values[1:] {
		if value < minValue {
			minValue = value
		}
		if value > maxValue {
			maxValue = value
		}
	}
	return math.Abs(maxValue - minValue)
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
