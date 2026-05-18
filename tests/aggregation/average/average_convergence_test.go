package average_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"sdcc-project/internal/config"
	"sdcc-project/internal/gossip"
	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
	shared "sdcc-project/internal/types"
)

// deterministicTransport è uno stub transport con consegna sincrona in-memory.
type deterministicTransport struct {
	handler transport.MessageHandler
}

// Start registra l'handler di ricezione.
func (d *deterministicTransport) Start(_ context.Context, h transport.MessageHandler) error {
	d.handler = h
	return nil
}

// Send non usa la rete reale nei test di convergenza.
func (d *deterministicTransport) Send(context.Context, string, []byte) error { return nil }

// Close non richiede teardown reale nello stub.
func (d *deterministicTransport) Close() error { return nil }

// inject consegna direttamente il payload all'handler.
func (d *deterministicTransport) inject(ctx context.Context, payload []byte) error {
	if d.handler == nil {
		return fmt.Errorf("handler non inizializzato")
	}
	return d.handler(ctx, payload)
}

// testNode raggruppa engine e transport fake del nodo.
type testNode struct {
	eng *gossip.Engine
	tr  *deterministicTransport
}

// testHarness espone API deterministiche per setup/consegna messaggi.
type testHarness struct {
	nodes map[shared.NodeID]*testNode
}

// newTestHarness costruisce nodi average con ticker molto lento per evitare round automatici.
func newTestHarness(t *testing.T, ids []shared.NodeID) *testHarness {
	t.Helper()

	h := &testHarness{nodes: make(map[shared.NodeID]*testNode, len(ids))}
	baseMembershipTime := time.Date(2026, 3, 17, 9, 0, 0, 0, time.UTC)
	for _, id := range ids {
		tr := &deterministicTransport{}
		membershipSet := membership.NewSet()
		for _, peerID := range ids {
			membershipSet.Upsert(membership.Peer{
				NodeID:   string(peerID),
				Addr:     fmt.Sprintf("%s:7000", peerID),
				Status:   membership.Alive,
				LastSeen: baseMembershipTime,
			})
		}
		eng := gossip.NewEngine(string(id), "average", tr, membershipSet, slog.Default(), nil, 24*time.Hour, 2)
		eng.State.EnsureMergeMetadata()
		eng.State.EnsureAverageMetadata()

		ctx, cancel := context.WithCancel(context.Background())
		localEng := eng
		localCancel := cancel
		t.Cleanup(func() {
			localCancel()
			_ = localEng.Stop()
		})

		if err := eng.Start(ctx); err != nil {
			t.Fatalf("start engine %s: %v", id, err)
		}
		h.nodes[id] = &testNode{eng: eng, tr: tr}
	}
	return h
}

// setLocalContribution imposta contributo locale iniziale (sum/count) del nodo.
func (h *testHarness) setLocalContribution(id shared.NodeID, sum float64, count uint64) {
	n := h.nodes[id]
	n.eng.State.NodeID = id
	n.eng.State.AggregationType = "average"
	n.eng.State.LocalValue = sum
	n.eng.State.Round = 0
	n.eng.State.VersionCounter = 0
	n.eng.State.UpdatedAt = time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)
	n.eng.State.AggregationData.Average.Contributions[id] = shared.AverageContribution{Sum: sum, Count: count}
	n.eng.State.AggregationData.Average.Versions[id] = shared.StateVersionStamp{Counter: 0}
	n.eng.State.Value = safeAverage(n.eng.State.AggregationData.Average.Contributions)
}

// deliverContribution invia un singolo contributo average dal nodo from al nodo to.
func (h *testHarness) deliverContribution(t *testing.T, from, to shared.NodeID, messageID shared.MessageID, version shared.StateVersion, sum float64, count uint64, sentAt time.Time) {
	t.Helper()

	msg := shared.GossipMessage{
		MessageID:    messageID,
		OriginNode:   from,
		SentAt:       sentAt,
		Version:      shared.MessageVersion{Major: 1, Minor: 0},
		StateVersion: shared.StateVersionStamp{Counter: version},
		State: shared.GossipState{
			NodeID:          from,
			AggregationType: "average",
			Value:           valueForMessage(sum, count),
			Round:           version,
			VersionCounter:  version,
			UpdatedAt:       sentAt,
			AggregationData: shared.AggregationState{Average: &shared.AverageState{
				Contributions: map[shared.NodeID]shared.AverageContribution{from: {Sum: sum, Count: count}},
				Versions:      map[shared.NodeID]shared.StateVersionStamp{from: {Counter: version}},
			}},
		},
	}

	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal messaggio: %v", err)
	}
	if err := h.nodes[to].tr.inject(context.Background(), raw); err != nil {
		t.Fatalf("inject %s->%s: %v", from, to, err)
	}
}

// assertNodeValue verifica il valore medio finale calcolato dal nodo.
func (h *testHarness) assertNodeValue(t *testing.T, id shared.NodeID, want float64) {
	t.Helper()
	got := h.nodes[id].eng.State.Value
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("valore inatteso su %s: got=%v want=%v", id, got, want)
	}
}

// TestAverageConvergence verifica convergenza robusta average su scenari distribuiti e edge case.
func TestAverageConvergence(t *testing.T) {
	base := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	t.Run("convergenza multi-nodo", func(t *testing.T) {
		ids := []shared.NodeID{"node-1", "node-2", "node-3"}
		h := newTestHarness(t, ids)
		h.setLocalContribution("node-1", 10, 1)
		h.setLocalContribution("node-2", 30, 1)
		h.setLocalContribution("node-3", 50, 1)

		versionByReceiver := map[shared.NodeID]shared.StateVersion{}
		for _, to := range ids {
			for _, from := range ids {
				if from == to {
					continue
				}
				versionByReceiver[to] += 10
				contribution := h.nodes[from].eng.State.AggregationData.Average.Contributions[from]
				h.deliverContribution(t, from, to, shared.MessageID(fmt.Sprintf("%s-to-%s-v%d", from, to, versionByReceiver[to])), versionByReceiver[to], contribution.Sum, contribution.Count, base)
			}
		}

		for _, id := range ids {
			h.assertNodeValue(t, id, 30)
		}
		for _, id := range ids {
			contribution := h.nodes[id].eng.State.AggregationData.Average.Contributions[id]
			if math.Abs(contribution.Sum-h.nodes[id].eng.State.LocalValue) > 1e-9 || contribution.Count != 1 {
				t.Fatalf("contributo locale driftato su %s: got=%+v local=%v", id, contribution, h.nodes[id].eng.State.LocalValue)
			}
		}
	})

	t.Run("duplicate update", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-2"})
		h.setLocalContribution("node-1", 10, 1)
		h.setLocalContribution("node-2", 40, 1)

		h.deliverContribution(t, "node-2", "node-1", "dup-node2-v1", 1, 40, 1, base.Add(1*time.Minute))
		first := h.nodes["node-1"].eng.State.Value
		h.deliverContribution(t, "node-2", "node-1", "dup-node2-v1", 1, 40, 1, base.Add(1*time.Minute))
		second := h.nodes["node-1"].eng.State.Value

		if math.Abs(first-second) > 1e-9 {
			t.Fatalf("duplicate update non idempotente: first=%v second=%v", first, second)
		}
		if math.Abs(second-25) > 1e-9 {
			t.Fatalf("media inattesa dopo duplicate update: got=%v want=25", second)
		}
	})

	t.Run("out-of-order", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-3"})
		h.setLocalContribution("node-1", 10, 1)
		h.setLocalContribution("node-3", 30, 1)

		h.deliverContribution(t, "node-3", "node-1", "node3-v5", 5, 50, 2, base.Add(2*time.Minute))
		afterNew := h.nodes["node-1"].eng.State.Value
		h.deliverContribution(t, "node-3", "node-1", "node3-v4-stale", 4, 5, 1, base.Add(3*time.Minute))
		afterStale := h.nodes["node-1"].eng.State.Value

		if math.Abs(afterNew-afterStale) > 1e-9 {
			t.Fatalf("messaggio stale ha alterato la media: new=%v stale=%v", afterNew, afterStale)
		}
		if math.Abs(afterStale-20) > 1e-9 {
			t.Fatalf("media inattesa dopo out-of-order: got=%v want=20", afterStale)
		}
	})

	t.Run("nodo lento", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-1", "node-2", "node-4"})
		h.setLocalContribution("node-1", 10, 1)
		h.setLocalContribution("node-2", 20, 1)
		h.setLocalContribution("node-4", 40, 1)

		h.deliverContribution(t, "node-1", "node-2", "node1-v1", 1, 10, 1, base)
		if got := h.nodes["node-2"].eng.State.Value; math.Abs(got-15) > 1e-9 {
			t.Fatalf("baseline inattesa senza nodo lento: got=%v want=15", got)
		}

		h.deliverContribution(t, "node-4", "node-2", "node4-v3-delayed", 3, 40, 1, base.Add(250*time.Millisecond))
		if got := h.nodes["node-2"].eng.State.Value; math.Abs(got-(70.0/3.0)) > 1e-9 {
			t.Fatalf("update del nodo lento non applicato: got=%v want=%v", got, 70.0/3.0)
		}
	})

	t.Run("filtro membership senza rete reale", func(t *testing.T) {
		ids := []shared.NodeID{"node-1", "node-2", "node-3", "node-4", "node-5", "node-6"}
		h := newTestHarness(t, ids)
		for index, value := range []float64{10, 30, 50, 70, 90, 110} {
			id := ids[index]
			h.setLocalContribution(id, value, 1)
		}

		for index, from := range ids[1:] {
			contribution := h.nodes[from].eng.State.AggregationData.Average.Contributions[from]
			version := shared.StateVersion((index + 1) * 10)
			h.deliverContribution(t, from, "node-1", shared.MessageID(fmt.Sprintf("%s-to-node-1", from)), version, contribution.Sum, contribution.Count, base)
		}
		h.assertNodeValue(t, "node-1", 60)

		node := h.nodes["node-1"]
		for _, activeID := range []string{"node-2", "node-3", "node-4"} {
			node.eng.Membership.Touch(activeID, time.Now().UTC())
		}
		node.eng.Membership.LeaveAt("node-5", time.Now().UTC())
		node.eng.Membership.Upsert(membership.Peer{NodeID: "node-6", Addr: "node-6:7000", Status: membership.Dead, LastSeen: time.Now().UTC()})
		node.eng.RoundOnce(context.Background())
		h.assertNodeValue(t, "node-1", 40)

		if _, retained := node.eng.State.AggregationData.Average.Contributions["node-5"]; !retained {
			t.Fatalf("contributo node-5 rimosso invece di essere solo filtrato")
		}
		if _, retained := node.eng.State.AggregationData.Average.Contributions["node-6"]; !retained {
			t.Fatalf("contributo node-6 rimosso invece di essere solo filtrato")
		}

		for _, activeID := range []string{"node-2", "node-3", "node-4", "node-5", "node-6"} {
			node.eng.Membership.Touch(activeID, time.Now().UTC())
		}
		node.eng.RoundOnce(context.Background())
		h.assertNodeValue(t, "node-1", 60)
	})

	t.Run("casi edge divisione per zero e stato vuoto", func(t *testing.T) {
		h := newTestHarness(t, []shared.NodeID{"node-a"})
		h.setLocalContribution("node-a", 0, 0)
		if got := h.nodes["node-a"].eng.State.Value; got != 0 {
			t.Fatalf("count zero dovrebbe produrre media zero: got=%v", got)
		}

		h.deliverContribution(t, "node-a", "node-a", "self-empty-v1", 1, 0, 0, base.Add(10*time.Minute))
		if got := h.nodes["node-a"].eng.State.Value; got != 0 {
			t.Fatalf("stato vuoto dovrebbe restare a zero: got=%v", got)
		}
	})
}

// TestAverageRoundDoesNotDriftLocalContribution congela la regressione in cui round successivi
// riscrivevano il contributo locale del nodo con la media corrente del cluster.
func TestAverageRoundDoesNotDriftLocalContribution(t *testing.T) {
	h := newTestHarness(t, []shared.NodeID{"node-1"})
	h.setLocalContribution("node-1", 10, 1)

	n := h.nodes["node-1"]
	n.eng.State.AggregationData.Average.Contributions["node-2"] = shared.AverageContribution{Sum: 30, Count: 1}
	n.eng.State.AggregationData.Average.Contributions["node-3"] = shared.AverageContribution{Sum: 50, Count: 1}
	n.eng.State.AggregationData.Average.Versions["node-2"] = shared.StateVersionStamp{Counter: 1}
	n.eng.State.AggregationData.Average.Versions["node-3"] = shared.StateVersionStamp{Counter: 1}
	n.eng.Membership.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: time.Now().UTC()})
	n.eng.Membership.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Alive, LastSeen: time.Now().UTC()})
	n.eng.State.Value = 30

	for round := 0; round < 4; round++ {
		n.eng.RoundOnce(context.Background())
	}

	localContribution := n.eng.State.AggregationData.Average.Contributions["node-1"]
	if math.Abs(localContribution.Sum-10) > 1e-9 || localContribution.Count != 1 {
		t.Fatalf("il contributo locale e' driftato dopo round multipli: got=%+v", localContribution)
	}
	if math.Abs(n.eng.State.Value-30) > 1e-9 {
		t.Fatalf("la media cluster attesa non e' stata preservata: got=%v want=30", n.eng.State.Value)
	}
}

// routedTransport instrada i messaggi gossip verso altri transport in-memory usando
// l'advertise_addr di configurazione come chiave di routing, senza aprire socket reali.
type routedTransport struct {
	addr    string
	router  *inMemoryRouter
	handler transport.MessageHandler
}

// Start registra l'handler locale e rende il nodo raggiungibile nel router condiviso.
func (r *routedTransport) Start(_ context.Context, h transport.MessageHandler) error {
	r.handler = h
	r.router.register(r.addr, r)
	return nil
}

// Send consegna sincronicamente il payload al nodo associato all'indirizzo di destinazione.
func (r *routedTransport) Send(ctx context.Context, addr string, payload []byte) error {
	return r.router.deliver(ctx, addr, payload)
}

// Close rimuove il transport dal router condiviso per evitare riusi accidentali tra test.
func (r *routedTransport) Close() error {
	r.router.unregister(r.addr)
	return nil
}

// inMemoryRouter conserva la tabella address -> transport usata dallo scenario a sei nodi.
type inMemoryRouter struct {
	mu     sync.RWMutex
	routes map[string]*routedTransport
}

// newInMemoryRouter inizializza una tabella di routing vuota e isolata per test.
func newInMemoryRouter() *inMemoryRouter {
	return &inMemoryRouter{routes: make(map[string]*routedTransport)}
}

// register pubblica un transport usando l'indirizzo applicativo configurato.
func (r *inMemoryRouter) register(addr string, tr *routedTransport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[addr] = tr
}

// unregister elimina una route, rendendo il teardown idempotente.
func (r *inMemoryRouter) unregister(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.routes, addr)
}

// deliver copia il payload prima di invocare l'handler remoto, così ogni consegna
// simula un pacchetto indipendente e non condivide buffer mutabili tra nodi.
func (r *inMemoryRouter) deliver(ctx context.Context, addr string, payload []byte) error {
	r.mu.RLock()
	target := r.routes[addr]
	r.mu.RUnlock()
	if target == nil || target.handler == nil {
		return fmt.Errorf("route gossip assente per %s", addr)
	}
	copied := append([]byte(nil), payload...)
	return target.handler(ctx, copied)
}

// configClusterNode rappresenta un nodo test costruito dai file configs/node*.yaml.
type configClusterNode struct {
	cfg config.Config
	eng *gossip.Engine
}

// TestAverageSixNodeClusterFromCanonicalConfigs verifica lo scenario richiesto a sei nodi
// usando i file canonici configs/node1.yaml ... configs/node6.yaml e membership stabile.
func TestAverageSixNodeClusterFromCanonicalConfigs(t *testing.T) {
	clearConfigOverrideEnv(t)

	cfgPaths := []string{
		repoPathForAverageTest(t, "configs", "node1.yaml"),
		repoPathForAverageTest(t, "configs", "node2.yaml"),
		repoPathForAverageTest(t, "configs", "node3.yaml"),
		repoPathForAverageTest(t, "configs", "node4.yaml"),
		repoPathForAverageTest(t, "configs", "node5.yaml"),
		repoPathForAverageTest(t, "configs", "node6.yaml"),
	}
	nodes := newConfigBackedCluster(t, cfgPaths)
	ids := sortedConfigClusterIDs(nodes)

	// Ventiquattro round sono volutamente superiori al minimo teorico: coprono più
	// finestre fanout anche per i nodi con seed parziali e rendono stabile sia la
	// membership canonica sia i contributi average appresi transitivamente.
	for round := 0; round < 24; round++ {
		for _, id := range ids {
			nodes[id].eng.RoundOnce(context.Background())
		}
	}

	for _, id := range ids {
		assertSixNodeCanonicalAverageState(t, nodes[id])
	}
}

// newConfigBackedCluster avvia un cluster in-memory rispettando bootstrap, fanout,
// timeout membership, aggregation e initial_value definiti nei file di configurazione.
func newConfigBackedCluster(t *testing.T, cfgPaths []string) map[shared.NodeID]*configClusterNode {
	t.Helper()

	router := newInMemoryRouter()
	nodes := make(map[shared.NodeID]*configClusterNode, len(cfgPaths))
	for _, cfgPath := range cfgPaths {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			t.Fatalf("load config %s: %v", cfgPath, err)
		}
		if cfg.Aggregation != "average" {
			t.Fatalf("config %s usa aggregation=%q, atteso average", cfgPath, cfg.Aggregation)
		}

		now := time.Now().UTC()
		mset := membership.NewSetWithConfig(cfg.MembershipConfig())
		mset.SetSelfNodeID(cfg.NodeID)
		mset.TouchOrUpsertCanonical(cfg.NodeID, cfg.AdvertiseEndpoint(), now)
		membership.Bootstrap(
			context.Background(),
			mset,
			membership.JoinRequest{NodeID: cfg.NodeID, Addr: cfg.AdvertiseEndpoint()},
			cfg.JoinEndpoint,
			cfg.DiscoveryPeers(),
			membership.NoopJoinClient{},
			now,
		)

		tr := &routedTransport{addr: cfg.AdvertiseEndpoint(), router: router}
		eng := gossip.NewEngine(cfg.NodeID, cfg.Aggregation, tr, mset, slog.Default(), nil, 24*time.Hour, cfg.Fanout)
		eng.State.LocalValue = cfg.InitialValue
		eng.State.Value = cfg.InitialValue
		eng.State.EnsureMergeMetadata()
		eng.State.EnsureAverageMetadata()

		ctx, cancel := context.WithCancel(context.Background())
		localEng := eng
		t.Cleanup(func() {
			cancel()
			_ = localEng.Stop()
		})
		if err := eng.Start(ctx); err != nil {
			t.Fatalf("start engine %s: %v", cfg.NodeID, err)
		}

		nodes[shared.NodeID(cfg.NodeID)] = &configClusterNode{cfg: cfg, eng: eng}
	}
	return nodes
}

// assertSixNodeCanonicalAverageState applica tutti gli invarianti richiesti dallo
// scenario: sei peer, sei contributi canonici, stima 60 e assenza di medie parziali.
func assertSixNodeCanonicalAverageState(t *testing.T, node *configClusterNode) {
	t.Helper()

	if got := len(node.eng.Membership.Snapshot()); got != 6 {
		t.Fatalf("%s peers=%d, atteso peers=6", node.cfg.NodeID, got)
	}

	contributions := node.eng.State.AggregationData.Average.Contributions
	canonical := canonicalAverageContributionIDs(contributions)
	if len(canonical) != 6 {
		t.Fatalf("%s conosce %d contributi average canonici (%v), attesi 6", node.cfg.NodeID, len(canonical), canonical)
	}
	for _, partial := range []float64{70, 76.666, 90} {
		if math.Abs(node.eng.State.Value-partial) <= 1e-3 {
			t.Fatalf("%s e' rimasto bloccato sulla media parziale %v", node.cfg.NodeID, node.eng.State.Value)
		}
	}
	if math.Abs(node.eng.State.Value-60) > 1e-9 {
		t.Fatalf("%s estimate=%v, atteso 60", node.cfg.NodeID, node.eng.State.Value)
	}
}

// canonicalAverageContributionIDs restituisce solo le chiavi logiche node-* ordinate,
// escludendo eventuali placeholder seed nel formato host:port.
func canonicalAverageContributionIDs(contributions map[shared.NodeID]shared.AverageContribution) []string {
	ids := make([]string, 0, len(contributions))
	for id := range contributions {
		text := string(id)
		if strings.HasPrefix(text, "node-") {
			ids = append(ids, text)
		}
	}
	sort.Strings(ids)
	return ids
}

// sortedConfigClusterIDs rende deterministico l'ordine dei round nel cluster test.
func sortedConfigClusterIDs(nodes map[shared.NodeID]*configClusterNode) []shared.NodeID {
	ids := make([]shared.NodeID, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// repoPathForAverageTest risolve percorsi dal repository root indipendentemente
// dalla working directory usata da `go test` per il package average.
func repoPathForAverageTest(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller non disponibile")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	return filepath.Join(append([]string{root}, parts...)...)
}

// clearConfigOverrideEnv impedisce a variabili d'ambiente esterne di alterare i
// file configs/node*.yaml durante questa verifica di scenario canonico.
func clearConfigOverrideEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"NODE_ID", "BIND_ADDRESS", "ADVERTISE_ADDR", "NODE_PORT", "JOIN_ENDPOINT",
		"BOOTSTRAP_PEERS", "SEED_PEERS", "GOSSIP_INTERVAL_MS", "FANOUT",
		"MEMBERSHIP_TIMEOUT_MS", "ENABLED_AGGREGATIONS", "AGGREGATION",
		"INITIAL_VALUE", "LOG_LEVEL",
	} {
		t.Setenv(name, "")
	}
}

// safeAverage calcola la media ignorando contributi con count zero.
func safeAverage(contributions map[shared.NodeID]shared.AverageContribution) float64 {
	totalSum := 0.0
	totalCount := uint64(0)
	for _, contribution := range contributions {
		if contribution.Count == 0 {
			continue
		}
		totalSum += contribution.Sum
		totalCount += contribution.Count
	}
	if totalCount == 0 {
		return 0
	}
	return totalSum / float64(totalCount)
}

// valueForMessage valorizza State.Value in modo coerente con sum/count del contributo.
func valueForMessage(sum float64, count uint64) float64 {
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
