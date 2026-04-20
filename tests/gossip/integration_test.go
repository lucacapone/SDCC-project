package gossip

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"

	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
)

type inMemoryNetwork struct {
	mu         sync.RWMutex
	transports map[string]*inMemoryTransport
	down       map[string]bool
}

func newInMemoryNetwork() *inMemoryNetwork {
	return &inMemoryNetwork{
		transports: make(map[string]*inMemoryTransport),
		down:       make(map[string]bool),
	}
}

func (n *inMemoryNetwork) newTransport(address string) *inMemoryTransport {
	return &inMemoryTransport{address: address, network: n}
}

func (n *inMemoryNetwork) register(address string, t *inMemoryTransport) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.transports[address] = t
}

func (n *inMemoryNetwork) unregister(address string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.transports, address)
}

func (n *inMemoryNetwork) setDown(address string, down bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.down[address] = down
}

func (n *inMemoryNetwork) deliver(ctx context.Context, to string, payload []byte) error {
	n.mu.RLock()
	dest := n.transports[to]
	down := n.down[to]
	n.mu.RUnlock()
	if down {
		return errors.New("nodo down")
	}
	if dest == nil {
		return errors.New("peer non registrato")
	}
	return dest.handle(ctx, payload)
}

type inMemoryTransport struct {
	address string
	network *inMemoryNetwork

	mu      sync.RWMutex
	handler transport.MessageHandler
	closed  bool
}

func (t *inMemoryTransport) Start(_ context.Context, handler transport.MessageHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handler = handler
	t.closed = false
	t.network.register(t.address, t)
	return nil
}

func (t *inMemoryTransport) Send(ctx context.Context, to string, payload []byte) error {
	t.mu.RLock()
	closed := t.closed
	t.mu.RUnlock()
	if closed {
		return errors.New("transport chiuso")
	}
	copyPayload := append([]byte(nil), payload...)
	return t.network.deliver(ctx, to, copyPayload)
}

func (t *inMemoryTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	t.network.unregister(t.address)
	return nil
}

func (t *inMemoryTransport) handle(ctx context.Context, payload []byte) error {
	t.mu.RLock()
	h := t.handler
	t.mu.RUnlock()
	if h == nil {
		return errors.New("handler non inizializzato")
	}
	return h(ctx, payload)
}

func TestIntegrationGossipConvergence(t *testing.T) {
	network := newInMemoryNetwork()
	nodes, cancel := startCluster(t, network, []float64{10, 30, 50}, 10*time.Millisecond)
	defer cancel()
	defer stopNodes(t, nodes...)

	if !waitConvergence(nodes, 2*time.Second, 0.05) {
		t.Fatalf("cluster non convergente entro timeout; valori=%v", values(nodes))
	}
}

func TestCrashNodeDownClusterResidualConverges(t *testing.T) {
	network := newInMemoryNetwork()
	nodes, cancel := startCluster(t, network, []float64{10, 30, 90}, 10*time.Millisecond)
	defer cancel()
	defer stopNodes(t, nodes...)

	network.setDown("node-3", true)
	if err := nodes[2].Stop(); err != nil {
		t.Fatalf("stop nodo 3: %v", err)
	}

	residual := []*Engine{nodes[0], nodes[1]}
	if !waitConvergence(residual, 2*time.Second, 0.05) {
		t.Fatalf("cluster residuo non convergente; valori=%v", values(residual))
	}
}

func TestCrashRestartRejoinOptional(t *testing.T) {
	network := newInMemoryNetwork()
	nodes, cancel := startCluster(t, network, []float64{10, 30, 90}, 10*time.Millisecond)
	defer cancel()
	defer stopNodes(t, nodes...)

	network.setDown("node-3", true)
	if err := nodes[2].Stop(); err != nil {
		t.Fatalf("stop nodo 3: %v", err)
	}

	residual := []*Engine{nodes[0], nodes[1]}
	if !waitConvergence(residual, 2*time.Second, 0.05) {
		t.Fatalf("cluster residuo non convergente prima del rejoin")
	}

	rejoinTransport := network.newTransport("node-3")
	rejoined := NewEngine("node-3", "average", rejoinTransport, fullMeshMembership("node-3", []string{"node-1", "node-2"}), slog.Default(), nil, 10*time.Millisecond, 2)
	rejoined.State.Value = 90
	if err := rejoined.Start(context.Background()); err != nil {
		t.Fatalf("start rejoin: %v", err)
	}
	defer stopNodes(t, rejoined)
	network.setDown("node-3", false)

	all := []*Engine{nodes[0], nodes[1], rejoined}
	if !waitConvergence(all, 2*time.Second, 0.08) {
		t.Fatalf("cluster non convergente dopo rejoin; valori=%v", values(all))
	}
}

func startCluster(t *testing.T, network *inMemoryNetwork, initialValues []float64, roundEvery time.Duration) ([]*Engine, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	nodes := make([]*Engine, 0, len(initialValues))
	addresses := make([]string, 0, len(initialValues))
	for i := range initialValues {
		addresses = append(addresses, nodeAddress(i+1))
	}

	for i, val := range initialValues {
		addr := nodeAddress(i + 1)
		peers := make([]string, 0, len(addresses)-1)
		for _, candidate := range addresses {
			if candidate != addr {
				peers = append(peers, candidate)
			}
		}
		engine := NewEngine(addr, "average", network.newTransport(addr), fullMeshMembership(addr, peers), slog.Default(), nil, roundEvery, 2)
		engine.State.Value = val
		if err := engine.Start(ctx); err != nil {
			cancel()
			stopNodes(t, nodes...)
			t.Fatalf("start nodo %s: %v", addr, err)
		}
		nodes = append(nodes, engine)
	}
	return nodes, cancel
}

func fullMeshMembership(self string, peers []string) *membership.Set {
	m := membership.NewSet()
	now := time.Now().UTC()
	for _, p := range peers {
		if p == self {
			continue
		}
		m.Join(p, now)
	}
	return m
}

func stopNodes(t *testing.T, nodes ...*Engine) {
	t.Helper()
	for _, n := range nodes {
		if n == nil {
			continue
		}
		_ = n.Stop()
	}
}

func waitConvergence(nodes []*Engine, timeout time.Duration, delta float64) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		vals := values(nodes)
		if maxDiff(vals) <= delta {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func values(nodes []*Engine) []float64 {
	vals := make([]float64, 0, len(nodes))
	for _, n := range nodes {
		vals = append(vals, n.State.Value)
	}
	return vals
}

func maxDiff(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	minV := vals[0]
	maxV := vals[0]
	for _, v := range vals[1:] {
		minV = math.Min(minV, v)
		maxV = math.Max(maxV, v)
	}
	return maxV - minV
}

func nodeAddress(i int) string {
	return fmt.Sprintf("node-%d", i)
}
