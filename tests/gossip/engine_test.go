package gossip

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"sdcc-project/internal/membership"
	"sdcc-project/internal/observability"
	"sdcc-project/internal/transport"
	shared "sdcc-project/internal/types"
)

func TestEngineStartStop(t *testing.T) {
	eng := NewEngine(
		"node-1",
		"sum",
		transport.NoopTransport{},
		membership.NewSet(),
		slog.Default(),
		nil,
		10*time.Millisecond,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start errore: %v", err)
	}
	if err := eng.Stop(); err != nil {
		t.Fatalf("stop errore: %v", err)
	}
}

type captureTransport struct {
	sent [][]byte
}

func (c *captureTransport) Start(context.Context, transport.MessageHandler) error { return nil }

func (c *captureTransport) Send(_ context.Context, _ string, payload []byte) error {
	c.sent = append(c.sent, append([]byte(nil), payload...))
	return nil
}

func (c *captureTransport) Close() error { return nil }

func TestRoundMessageAndStateVersionAlignment(t *testing.T) {
	tr := &captureTransport{}
	m := membership.NewSet()
	m.Join("node-2", time.Now().UTC())

	eng := NewEngine("node-1", "average", tr, m, slog.Default(), nil, time.Second)
	eng.State.VersionCounter = 2
	eng.State.Round = 2

	eng.RoundOnce(context.Background())

	if len(tr.sent) != 1 {
		t.Fatalf("messaggi inviati inattesi: got=%d want=1", len(tr.sent))
	}

	var msg shared.GossipMessage
	if err := json.Unmarshal(tr.sent[0], &msg); err != nil {
		t.Fatalf("unmarshal messaggio: %v", err)
	}

	if msg.StateVersion != normalizeVersion(msg.State) {
		t.Fatalf("state_version non allineata allo stato serializzato: got=%+v state=%+v", msg.StateVersion, normalizeVersion(msg.State))
	}
	if msg.StateVersion.Counter != 3 {
		t.Fatalf("counter messaggio inatteso: got=%d want=3", msg.StateVersion.Counter)
	}
	if msg.State.Round != 3 {
		t.Fatalf("round messaggio inatteso: got=%d want=3", msg.State.Round)
	}
	if eng.State.VersionCounter != msg.StateVersion.Counter {
		t.Fatalf("versione locale non allineata al messaggio: local=%d msg=%d", eng.State.VersionCounter, msg.StateVersion.Counter)
	}
	if eng.State.Round != msg.State.Round {
		t.Fatalf("round locale non allineato al messaggio: local=%d msg=%d", eng.State.Round, msg.State.Round)
	}
}

func TestRoundLoggingEsponeCampiStabili(t *testing.T) {
	tr := &captureTransport{}
	m := membership.NewSet()
	now := time.Now().UTC()
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: now})
	m.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Suspect, LastSeen: now})

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	eng := NewEngine("node-1", "average", tr, m, logger, nil, time.Second)
	eng.State.Value = 42.5

	eng.RoundOnce(context.Background())

	logged := logBuffer.String()
	for _, expected := range []string{
		"event=gossip_round",
		"node_id=node-1",
		"round=1",
		"peers=2",
		"estimate=42.5",
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log round privo del campo atteso %q: %s", expected, logged)
		}
	}
}

func TestRoundNonLoggaTimeoutPerSelfNode(t *testing.T) {
	tr := &captureTransport{}
	base := time.Now().UTC()
	m := membership.NewSetWithConfig(membership.Config{
		SuspectTimeout: time.Second,
		DeadTimeout:    2 * time.Second,
		PruneRetention: 20 * time.Second,
	})
	m.Upsert(membership.Peer{NodeID: "node-1", Addr: "node-1:7001", Status: membership.Alive, LastSeen: base.Add(-3 * time.Second)})
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: base.Add(-3 * time.Second)})

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
	eng := NewEngine("node-1", "sum", tr, m, logger, nil, time.Hour)

	eng.RoundOnce(context.Background())

	snapshot := membershipByNodeID(m.Snapshot())
	if snapshot["node-1"].Status != membership.Alive {
		t.Fatalf("self node non deve degradare nel round: got=%s", snapshot["node-1"].Status)
	}
	if snapshot["node-2"].Status != membership.Dead {
		t.Fatalf("peer remoto deve degradare per timeout: got=%s", snapshot["node-2"].Status)
	}

	logged := logBuffer.String()
	if strings.Contains(logged, "peer_id=node-1") {
		t.Fatalf("log timeout non deve includere self node: %s", logged)
	}
	if !strings.Contains(logged, "event=membership_transition") || !strings.Contains(logged, "peer_id=node-2") {
		t.Fatalf("log timeout atteso per peer remoto mancante: %s", logged)
	}
}

func TestRoundNonLoggaMembershipTransitionPerAliasDelNodoLocale(t *testing.T) {
	tr := &captureTransport{}
	base := time.Now().UTC().Add(-4 * time.Second)
	m := membership.NewSetWithConfig(membership.Config{
		SuspectTimeout: time.Second,
		DeadTimeout:    2 * time.Second,
		PruneRetention: 20 * time.Second,
	})
	m.Upsert(membership.Peer{
		NodeID:      "node-3",
		Addr:        "node3:7003",
		Status:      membership.Alive,
		Incarnation: 7,
		LastSeen:    base,
	})

	mergeMembershipWithSelf(m, "node-3", []shared.MembershipEntry{
		{
			NodeID:      "node3:7003",
			Addr:        "node3:7003",
			Status:      string(membership.Alive),
			Incarnation: 99,
			LastSeen:    base,
		},
	}, "node3:7003")

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
	eng := NewEngine("node-3", "sum", tr, m, logger, nil, time.Hour)

	eng.RoundOnce(context.Background())

	snapshot := membershipByNodeID(m.Snapshot())
	if _, exists := snapshot["node3:7003"]; exists {
		t.Fatalf("alias del nodo locale non deve entrare nella membership: %+v", snapshot["node3:7003"])
	}
	if snapshot["node-3"].Status != membership.Alive {
		t.Fatalf("self canonico non deve degradare: got=%s", snapshot["node-3"].Status)
	}

	logged := logBuffer.String()
	if strings.Contains(logged, "event=membership_transition") && strings.Contains(logged, "peer_id=node3:7003") {
		t.Fatalf("transition auto-riferita inattesa per alias self: %s", logged)
	}
}

func TestAverageRoundPreservaContributoLocaleOriginario(t *testing.T) {
	tr := &captureTransport{}
	m := membership.NewSet()
	eng := NewEngine("node-1", "average", tr, m, slog.Default(), nil, time.Second)
	eng.State.LocalValue = 10
	eng.State.Value = 30
	eng.State.EnsureAverageMetadata()
	eng.State.AggregationData.Average.Contributions["node-1"] = shared.AverageContribution{Sum: 10, Count: 1}
	eng.State.AggregationData.Average.Contributions["node-2"] = shared.AverageContribution{Sum: 30, Count: 1}
	eng.State.AggregationData.Average.Contributions["node-3"] = shared.AverageContribution{Sum: 50, Count: 1}
	eng.State.AggregationData.Average.Versions["node-1"] = shared.StateVersionStamp{Counter: 1}
	eng.State.AggregationData.Average.Versions["node-2"] = shared.StateVersionStamp{Counter: 1}
	eng.State.AggregationData.Average.Versions["node-3"] = shared.StateVersionStamp{Counter: 1}

	eng.RoundOnce(context.Background())
	eng.RoundOnce(context.Background())

	localContribution := eng.State.AggregationData.Average.Contributions["node-1"]
	if localContribution != (shared.AverageContribution{Sum: 10, Count: 1}) {
		t.Fatalf("contributo locale riscritto impropriamente: got=%+v", localContribution)
	}
	if eng.State.Value != 30 {
		t.Fatalf("media cluster inattesa dopo round multipli: got=%v want=30", eng.State.Value)
	}
}

func TestRemoteMergeLoggingRiduceDettagliSensibiliAMetadataUtili(t *testing.T) {
	tr := &spyTransportEngine{}
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mset := membership.NewSet()
	eng := NewEngine("node-1", "sum", tr, mset, logger, nil, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	now := time.Unix(1710000000, 0).UTC()
	incoming := shared.GossipMessage{
		MessageID:  "m-merge-1",
		OriginNode: "node-2",
		SentAt:     now,
		Version:    currentMessageVersion,
		StateVersion: shared.StateVersionStamp{
			Epoch:   1,
			Counter: 1,
		},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "sum",
			Value:           99.5,
			VersionEpoch:    1,
			VersionCounter:  1,
			Round:           7,
			UpdatedAt:       now,
		},
		Membership: []shared.MembershipEntry{{
			NodeID:      "node-2",
			Addr:        "node-2:7002",
			Status:      string(membership.Alive),
			Incarnation: 2,
			LastSeen:    now,
		}},
	}
	payload, err := json.Marshal(incoming)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := tr.deliver(context.Background(), payload); err != nil {
		t.Fatalf("deliver handler: %v", err)
	}

	logged := logBuffer.String()
	for _, expected := range []string{
		"event=remote_merge",
		"node_id=node-1",
		"merge_status=applied",
		"merge_reason=remote_newer_version",
		"remote_node_id=node-2",
		"remote_round=7",
		"remote_estimate=99.5",
		"estimate=99.5",
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log merge privo del campo atteso %q: %s", expected, logged)
		}
	}

	for _, forbidden := range []string{"contributions", "versions", "SeenMessageIDs"} {
		if strings.Contains(logged, forbidden) {
			t.Fatalf("log merge contiene dettagli troppo verbosi %q: %s", forbidden, logged)
		}
	}
}

func TestRemoteMergeLoggingMantieneSeparatiPeersLocaliEMembershipEntries(t *testing.T) {
	tr := &spyTransportEngine{}
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mset := membership.NewSet()
	now := time.Unix(1710000000, 0).UTC()
	mset.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Alive, LastSeen: now})
	eng := NewEngine("node-1", "sum", tr, mset, logger, nil, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	incoming := shared.GossipMessage{
		MessageID:  "m-merge-semantics-1",
		OriginNode: "node-2",
		SentAt:     now,
		Version:    currentMessageVersion,
		StateVersion: shared.StateVersionStamp{
			Epoch:   1,
			Counter: 1,
		},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "sum",
			Value:           15,
			VersionEpoch:    1,
			VersionCounter:  1,
			Round:           3,
			UpdatedAt:       now,
		},
		Membership: nil,
	}
	payload, err := json.Marshal(incoming)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := tr.deliver(context.Background(), payload); err != nil {
		t.Fatalf("deliver handler: %v", err)
	}

	logged := logBuffer.String()
	for _, expected := range []string{
		"event=remote_merge",
		"peers=1",
		"membership_entries=0",
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log merge privo del campo atteso %q: %s", expected, logged)
		}
	}
}

func TestRoundAggiornaCollectorConValoriRuntime(t *testing.T) {
	tr := &captureTransport{}
	mset := membership.NewSet()
	now := time.Now().UTC()
	mset.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: now})
	collector := observability.NewCollector(now)
	eng := NewEngine("node-1", "sum", tr, mset, slog.Default(), collector, time.Second)
	eng.State.Value = 12.5

	eng.RoundOnce(context.Background())

	snapshot := collector.Snapshot(time.Now().UTC())
	if snapshot.TotalRounds != 1 {
		t.Fatalf("round osservati inattesi: got=%d want=1", snapshot.TotalRounds)
	}
	if snapshot.KnownPeers != 1 {
		t.Fatalf("peer osservati inattesi: got=%d want=1", snapshot.KnownPeers)
	}
	if snapshot.CurrentEstimate != eng.State.Value {
		t.Fatalf("stima osservata inattesa: got=%v want=%v", snapshot.CurrentEstimate, eng.State.Value)
	}
}

func TestRemoteMergeAggiornaCollectorConEsitoEStatoRuntime(t *testing.T) {
	tr := &spyTransportEngine{}
	mset := membership.NewSet()
	collector := observability.NewCollector(time.Now().UTC())
	eng := NewEngine("node-1", "sum", tr, mset, slog.Default(), collector, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	now := time.Unix(1710000000, 0).UTC()
	incoming := shared.GossipMessage{
		MessageID:  "m-merge-collector-1",
		OriginNode: "node-2",
		SentAt:     now,
		Version:    currentMessageVersion,
		StateVersion: shared.StateVersionStamp{
			Epoch:   1,
			Counter: 1,
		},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "sum",
			Value:           77.0,
			VersionEpoch:    1,
			VersionCounter:  1,
			Round:           5,
			UpdatedAt:       now,
		},
		Membership: []shared.MembershipEntry{{
			NodeID:      "node-2",
			Addr:        "node-2:7002",
			Status:      string(membership.Alive),
			Incarnation: 3,
			LastSeen:    now,
		}},
	}
	payload, err := json.Marshal(incoming)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := tr.deliver(context.Background(), payload); err != nil {
		t.Fatalf("deliver handler: %v", err)
	}

	snapshot := collector.Snapshot(time.Now().UTC())
	if snapshot.RemoteMerges["applied"] != 1 {
		t.Fatalf("merge applied osservati inattesi: got=%d want=1", snapshot.RemoteMerges["applied"])
	}
	if snapshot.KnownPeers != 1 {
		t.Fatalf("peer osservati inattesi dopo merge: got=%d want=1", snapshot.KnownPeers)
	}
	if snapshot.CurrentEstimate != 77.0 {
		t.Fatalf("stima osservata inattesa dopo merge: got=%v want=77", snapshot.CurrentEstimate)
	}
}
