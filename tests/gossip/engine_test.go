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
		2,
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
	addr []string
}

func (c *captureTransport) Start(context.Context, transport.MessageHandler) error { return nil }

func (c *captureTransport) Send(_ context.Context, target string, payload []byte) error {
	return c.sendTo(target, payload)
}

func (c *captureTransport) sendTo(target string, payload []byte) error {
	c.addr = append(c.addr, target)
	c.sent = append(c.sent, append([]byte(nil), payload...))
	return nil
}

func (c *captureTransport) Close() error { return nil }

type deterministicRNG struct {
	values []int
	index  int
}

func (d *deterministicRNG) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	if len(d.values) == 0 {
		return 0
	}
	value := d.values[d.index%len(d.values)] % n
	d.index++
	return value
}

func TestRoundMessageAndStateVersionAlignment(t *testing.T) {
	tr := &captureTransport{}
	m := membership.NewSet()
	m.Join("node-2", time.Now().UTC())

	eng := NewEngine("node-1", "average", tr, m, slog.Default(), nil, time.Second, 2)
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

func TestRoundFanoutOneInviaUnSoloPeer(t *testing.T) {
	tr := &captureTransport{}
	now := time.Now().UTC()
	m := membership.NewSet()
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: now})
	m.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Alive, LastSeen: now})

	eng := NewEngine("node-1", "sum", tr, m, slog.Default(), nil, time.Second, 1)
	eng.RNG = &deterministicRNG{values: []int{0}}

	eng.RoundOnce(context.Background())

	if len(tr.sent) != 1 {
		t.Fatalf("fanout=1 deve inviare un solo messaggio: got=%d", len(tr.sent))
	}
	if len(tr.addr) != 1 {
		t.Fatalf("fanout=1 deve registrare un solo destinatario: got=%d", len(tr.addr))
	}
}

func TestRoundFanoutMaggioreDeiPeerInviaATutti(t *testing.T) {
	tr := &captureTransport{}
	now := time.Now().UTC()
	m := membership.NewSet()
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: now})
	m.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Suspect, LastSeen: now})

	eng := NewEngine("node-1", "sum", tr, m, slog.Default(), nil, time.Second, 10)

	eng.RoundOnce(context.Background())

	if len(tr.sent) != 2 {
		t.Fatalf("fanout>peer deve inviare a tutti i peer eleggibili: got=%d want=2", len(tr.sent))
	}
}

func TestNewEngineFanoutNonPositivoNormalizzaAUno(t *testing.T) {
	tr := &captureTransport{}
	now := time.Now().UTC()
	m := membership.NewSet()
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: now})
	m.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Alive, LastSeen: now})

	eng := NewEngine("node-1", "sum", tr, m, slog.Default(), nil, time.Second, 0)
	eng.RNG = &deterministicRNG{values: []int{0}}

	if eng.Fanout != 1 {
		t.Fatalf("fanout non positivo deve essere normalizzato a 1: got=%d", eng.Fanout)
	}

	eng.RoundOnce(context.Background())
	if len(tr.sent) != 1 {
		t.Fatalf("fanout normalizzato deve inviare un solo messaggio: got=%d", len(tr.sent))
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
	eng := NewEngine("node-1", "average", tr, m, logger, nil, time.Second, 2)
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
	eng := NewEngine("node-1", "sum", tr, m, logger, nil, time.Hour, 2)

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
	eng := NewEngine("node-3", "sum", tr, m, logger, nil, time.Hour, 2)

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
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: time.Now().UTC()})
	m.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Alive, LastSeen: time.Now().UTC()})
	eng := NewEngine("node-1", "average", tr, m, slog.Default(), nil, time.Second, 2)
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
	eng := NewEngine("node-1", "sum", tr, mset, logger, nil, time.Hour, 2)

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
		"merge_reason=remote_contribution_merged",
		"node_decisions_newer_version=1",
		"node_decisions_duplicate_ignored=0",
		"node_decisions_tie_break=0",
		"remote_node_id=node-2",
		"remote_node_decision=newer_version",
		"remote_round=7",
		"remote_estimate=99.5",
		"estimate=99.5",
		"max_preserved=false",
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log merge privo del campo atteso %q: %s", expected, logged)
		}
	}

	for _, forbidden := range []string{"contributions", "versions", "SeenMessageIDs", "conflict_node_id=", "conflict_decision="} {
		if strings.Contains(logged, forbidden) {
			t.Fatalf("log merge contiene dettagli non attesi %q: %s", forbidden, logged)
		}
	}
}

func TestRemoteMergeLoggingInfoApplicatoMantieneSoloCampiBase(t *testing.T) {
	tr := &spyTransportEngine{}
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mset := membership.NewSet()
	eng := NewEngine("node-1", "sum", tr, mset, logger, nil, time.Hour, 2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	now := time.Unix(1710000000, 0).UTC()
	incoming := shared.GossipMessage{
		MessageID:  "m-info-base-1",
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
			Value:           42,
			VersionEpoch:    1,
			VersionCounter:  1,
			Round:           7,
			UpdatedAt:       now,
		},
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
		"round=",
		"peers=",
		"estimate=",
		"merge_status=applied",
		"remote_node_id=node-2",
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log INFO applicato privo del campo base %q: %s", expected, logged)
		}
	}
	for _, diagnostic := range []string{
		"estimate_before=",
		"estimate_after=",
		"remote_round=",
		"remote_estimate=",
		"membership_entries=",
		"unique_nodes=",
		"node_decisions_newer_version=",
		"node_decisions_duplicate_ignored=",
		"node_decisions_tie_break=",
		"remote_node_decision=",
		"max_preserved=",
		"merge_reason=",
		"conflict_node_id=",
		"conflict_decision=",
	} {
		if strings.Contains(logged, diagnostic) {
			t.Fatalf("log INFO applicato contiene campo diagnostico %q: %s", diagnostic, logged)
		}
	}
}

func TestRemoteMergeLoggingInfoConflittoMantieneDettagliCompleti(t *testing.T) {
	tr := &spyTransportEngine{}
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mset := membership.NewSet()
	eng := NewEngine("node-1", "sum", tr, mset, logger, nil, time.Hour, 2)
	eng.State.Value = 12
	eng.State.VersionCounter = 2
	eng.State.UpdatedAt = time.Unix(1710000000, 0).UTC()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	now := time.Unix(1710000001, 0).UTC()
	incoming := shared.GossipMessage{
		MessageID:  "m-conflict-info-1",
		OriginNode: "node-2",
		SentAt:     now,
		Version:    currentMessageVersion,
		StateVersion: shared.StateVersionStamp{
			Counter: 2,
		},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "average",
			Value:           99,
			VersionCounter:  2,
			Round:           5,
			UpdatedAt:       now,
		},
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
		"merge_status=conflict",
		"remote_node_id=node-2",
		"estimate_before=12",
		"estimate_after=12",
		"remote_round=5",
		"remote_estimate=99",
		"membership_entries=0",
		"unique_nodes=0",
		"node_decisions_newer_version=0",
		"node_decisions_duplicate_ignored=0",
		"node_decisions_tie_break=0",
		"remote_node_decision=not_present",
		"max_preserved=false",
		"merge_reason=aggregation_type_mismatch",
		"conflict_node_id=",
		"conflict_decision=",
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log INFO conflitto privo del dettaglio atteso %q: %s", expected, logged)
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
	eng := NewEngine("node-1", "sum", tr, mset, logger, nil, time.Hour, 2)

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

func TestRemoteMergeLoggingAverageDistingueContributiNotiEUsati(t *testing.T) {
	tr := &spyTransportEngine{}
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mset := membership.NewSet()
	now := time.Unix(1710000500, 0).UTC()
	mset.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Dead, LastSeen: now.Add(-time.Minute)})

	eng := NewEngine("node-1", "average", tr, mset, logger, nil, time.Hour, 2)
	eng.State.Value = 30
	eng.State.VersionCounter = 2
	eng.State.AggregationData.Average = &shared.AverageState{
		Contributions: map[shared.NodeID]shared.AverageContribution{
			"node-1": {Sum: 10, Count: 1},
			"node-3": {Sum: 90, Count: 1},
		},
		Versions: map[shared.NodeID]shared.StateVersionStamp{
			"node-1": {Counter: 2},
			"node-3": {Counter: 1},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	incoming := shared.GossipMessage{
		MessageID:  "m-average-log-1",
		OriginNode: "node-2",
		SentAt:     now,
		Version:    currentMessageVersion,
		StateVersion: shared.StateVersionStamp{
			Counter: 3,
		},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "average",
			Value:           30,
			VersionCounter:  3,
			Round:           4,
			UpdatedAt:       now,
			AggregationData: shared.AggregationState{Average: &shared.AverageState{
				Contributions: map[shared.NodeID]shared.AverageContribution{
					"node-2": {Sum: 30, Count: 1},
				},
				Versions: map[shared.NodeID]shared.StateVersionStamp{
					"node-2": {Counter: 3},
				},
			}},
		},
		Membership: []shared.MembershipEntry{
			{NodeID: "node-2", Addr: "node-2:7002", Status: string(membership.Alive), Incarnation: 1, LastSeen: now},
			{NodeID: "node-3", Addr: "node-3:7003", Status: string(membership.Dead), Incarnation: 2, LastSeen: now},
		},
	}
	payload, err := json.Marshal(incoming)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := tr.deliver(context.Background(), payload); err != nil {
		t.Fatalf("deliver handler: %v", err)
	}

	if got := eng.State.Value; got != 20 {
		t.Fatalf("media filtrata inattesa: got=%v want=20", got)
	}
	logged := logBuffer.String()
	for _, expected := range []string{
		"event=remote_merge",
		"merge_status=applied",
		"peers=2",
		"estimate=20",
		"average_known_contributions=3",
		"average_eligible_contributions=2",
		"average_eligible_node_ids=\"[node-1 node-2]\"",
		"average_contribution_node_ids=\"[node-1 node-2 node-3]\"",
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log average privo del campo atteso %q: %s", expected, logged)
		}
	}
}

func TestRemoteMergeSkippedConRicalcoloRuntimeDiventaPartialMerge(t *testing.T) {
	tr := &spyTransportEngine{}
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mset := membership.NewSet()
	now := time.Unix(1710000400, 0).UTC()
	mset.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Dead, LastSeen: now.Add(-time.Minute)})

	collector := observability.NewCollector(now)
	eng := NewEngine("node-1", "average", tr, mset, logger, collector, time.Hour, 2)
	eng.State.Value = 10
	eng.State.VersionCounter = 10
	eng.State.Round = 10
	eng.State.AggregationData.Average = &shared.AverageState{
		Contributions: map[shared.NodeID]shared.AverageContribution{
			"node-1": {Sum: 10, Count: 1},
			"node-2": {Sum: 30, Count: 1},
		},
		Versions: map[shared.NodeID]shared.StateVersionStamp{
			"node-1": {Counter: 10},
			"node-2": {Counter: 7},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	incoming := shared.GossipMessage{
		MessageID:  "m-older-runtime-partial-1",
		OriginNode: "node-2",
		SentAt:     now,
		Version:    currentMessageVersion,
		StateVersion: shared.StateVersionStamp{
			Counter: 7,
		},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "average",
			Value:           30,
			VersionCounter:  7,
			Round:           7,
			UpdatedAt:       now,
			AggregationData: shared.AggregationState{Average: &shared.AverageState{
				Contributions: map[shared.NodeID]shared.AverageContribution{
					"node-2": {Sum: 30, Count: 1},
				},
				Versions: map[shared.NodeID]shared.StateVersionStamp{
					"node-2": {Counter: 7},
				},
			}},
		},
	}
	payload, err := json.Marshal(incoming)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := tr.deliver(context.Background(), payload); err != nil {
		t.Fatalf("deliver handler: %v", err)
	}

	if eng.State.Value != 20 {
		t.Fatalf("stima runtime attesa dopo ricalcolo membership: got=%v want=20", eng.State.Value)
	}
	logged := logBuffer.String()
	for _, expected := range []string{
		"event=remote_merge",
		"merge_status=partial_merge",
		"merge_reason=older_version",
		"estimate_before=10",
		"estimate_after=20",
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log partial_merge privo del campo atteso %q: %s", expected, logged)
		}
	}
	if strings.Contains(logged, "merge_status=skipped") {
		t.Fatalf("merge con ricalcolo runtime non deve restare skipped: %s", logged)
	}
	snapshot := collector.Snapshot(time.Now().UTC())
	if snapshot.RemoteMerges["partial_merge"] != 1 {
		t.Fatalf("collector non conta partial_merge: %+v", snapshot.RemoteMerges)
	}
}

func TestRemoteMergeSelfOriginRestaNoOpESenzaRumoreInfo(t *testing.T) {
	tr := &spyTransportEngine{}
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mset := membership.NewSet()
	eng := NewEngine("node-1", "sum", tr, mset, logger, nil, time.Hour, 2)

	eng.State.Value = 17.25
	eng.State.Round = 4
	roundBefore := eng.State.Round
	estimateBefore := eng.State.Value

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	now := time.Unix(1710000200, 0).UTC()
	incoming := shared.GossipMessage{
		MessageID:  "m-self-origin-1",
		OriginNode: "node-1",
		SentAt:     now,
		Version:    currentMessageVersion,
		StateVersion: shared.StateVersionStamp{
			Epoch:   1,
			Counter: 8,
		},
		State: shared.GossipState{
			NodeID:          "node-1",
			AggregationType: "sum",
			Value:           99.5,
			VersionEpoch:    1,
			VersionCounter:  8,
			Round:           8,
			UpdatedAt:       now,
		},
	}
	payload, err := json.Marshal(incoming)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := tr.deliver(context.Background(), payload); err != nil {
		t.Fatalf("deliver handler: %v", err)
	}

	if eng.State.Round != roundBefore {
		t.Fatalf("round alterato da auto-merge: got=%d want=%d", eng.State.Round, roundBefore)
	}
	if eng.State.Value != estimateBefore {
		t.Fatalf("estimate alterata da auto-merge: got=%v want=%v", eng.State.Value, estimateBefore)
	}

	logged := logBuffer.String()
	if strings.Contains(logged, "event=remote_merge") {
		t.Fatalf("auto-merge non deve generare remote_merge a livello INFO: %s", logged)
	}
}

func TestRoundAggiornaCollectorConValoriRuntime(t *testing.T) {
	tr := &captureTransport{}
	mset := membership.NewSet()
	now := time.Now().UTC()
	mset.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: now})
	collector := observability.NewCollector(now)
	eng := NewEngine("node-1", "sum", tr, mset, slog.Default(), collector, time.Second, 2)
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

func TestRemoteMergeRicalcolaEstimateConMembershipAggiornata(t *testing.T) {
	tr := &spyTransportEngine{}
	now := time.Unix(1710000300, 0).UTC()
	mset := membership.NewSet()
	mset.Upsert(membership.Peer{NodeID: "node-1", Addr: "node-1:7001", Status: membership.Alive, Incarnation: 1, LastSeen: now})
	mset.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Alive, Incarnation: 1, LastSeen: now})
	collector := observability.NewCollector(now)
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	eng := NewEngine("node-1", "sum", tr, mset, logger, collector, time.Hour, 2)
	eng.State.LocalValue = 10
	eng.State.Value = 60
	eng.State.EnsureSumMetadata()
	eng.State.AggregationData.Sum.Contributions["node-1"] = 10
	eng.State.AggregationData.Sum.Contributions["node-3"] = 50
	eng.State.AggregationData.Sum.Versions["node-1"] = shared.StateVersionStamp{Counter: 1}
	eng.State.AggregationData.Sum.Versions["node-3"] = shared.StateVersionStamp{Counter: 1}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("start engine errore: %v", err)
	}
	defer eng.Stop()

	incoming := shared.GossipMessage{
		MessageID:  "m-merge-membership-aware-1",
		OriginNode: "node-2",
		SentAt:     now,
		Version:    currentMessageVersion,
		StateVersion: shared.StateVersionStamp{
			Epoch:   1,
			Counter: 2,
		},
		State: shared.GossipState{
			NodeID:          "node-2",
			AggregationType: "sum",
			Value:           30,
			VersionEpoch:    1,
			VersionCounter:  2,
			Round:           4,
			UpdatedAt:       now,
		},
		Membership: []shared.MembershipEntry{
			{NodeID: "node-2", Addr: "node-2:7002", Status: string(membership.Alive), Incarnation: 1, LastSeen: now},
			{NodeID: "node-3", Addr: "node-3:7003", Status: string(membership.Dead), Incarnation: 2, LastSeen: now},
		},
	}
	payload, err := json.Marshal(incoming)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := tr.deliver(context.Background(), payload); err != nil {
		t.Fatalf("deliver handler: %v", err)
	}

	if got := eng.State.Value; got != 40 {
		t.Fatalf("stima post-merge non ricalcolata sulla membership aggiornata: got=%v want=40", got)
	}
	if got := eng.State.AggregationData.Sum.Contributions["node-3"]; got != 50 {
		t.Fatalf("contributo del nodo dead non deve essere cancellato: got=%v", got)
	}
	snapshot := collector.Snapshot(time.Now().UTC())
	if snapshot.CurrentEstimate != 40 {
		t.Fatalf("collector non usa la stima ricalcolata: got=%v want=40", snapshot.CurrentEstimate)
	}
	logged := logBuffer.String()
	for _, expected := range []string{"estimate=40", "estimate_after=40", "unique_nodes=3"} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log merge privo della stima membership-aware %q: %s", expected, logged)
		}
	}
}

func TestRemoteMergeAggiornaCollectorConEsitoEStatoRuntime(t *testing.T) {
	tr := &spyTransportEngine{}
	mset := membership.NewSet()
	collector := observability.NewCollector(time.Now().UTC())
	eng := NewEngine("node-1", "sum", tr, mset, slog.Default(), collector, time.Hour, 2)

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

func TestEligibleNodeIDsIncludeSoloAliveESelfBootstrap(t *testing.T) {
	peers := []membership.Peer{
		{NodeID: "node-2", Status: membership.Alive},
		{NodeID: "node-3", Status: membership.Suspect},
		{NodeID: "node-4", Status: membership.Dead},
		{NodeID: "node-5", Status: membership.Left},
		{NodeID: "node-6"},
	}

	eligible := EligibleNodeIDsForTest("node-1", peers)
	if _, ok := eligible["node-1"]; !ok {
		t.Fatalf("self node assente dallo snapshot deve essere incluso")
	}
	if _, ok := eligible["node-2"]; !ok {
		t.Fatalf("peer alive deve essere incluso")
	}
	for _, nodeID := range []shared.NodeID{"node-3", "node-4", "node-5", "node-6"} {
		if _, ok := eligible[nodeID]; ok {
			t.Fatalf("peer non alive %s non deve essere incluso: %+v", nodeID, eligible)
		}
	}
}

func TestRoundSumFiltraContributiNonEleggibiliSenzaCancellarli(t *testing.T) {
	tr := &captureTransport{}
	m := membership.NewSet()
	m.Upsert(membership.Peer{NodeID: "node-1", Addr: "node-1:7001", Status: membership.Alive, LastSeen: time.Now().UTC()})
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: time.Now().UTC()})
	m.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Suspect, LastSeen: time.Now().UTC()})

	eng := NewEngine("node-1", "sum", tr, m, slog.Default(), nil, time.Hour, 3)
	eng.State.LocalValue = 10
	eng.State.EnsureSumMetadata()
	eng.State.AggregationData.Sum.Contributions["node-1"] = 10
	eng.State.AggregationData.Sum.Contributions["node-2"] = 30
	eng.State.AggregationData.Sum.Contributions["node-3"] = 50
	eng.State.AggregationData.Sum.Versions["node-1"] = shared.StateVersionStamp{Counter: 1}
	eng.State.AggregationData.Sum.Versions["node-2"] = shared.StateVersionStamp{Counter: 1}
	eng.State.AggregationData.Sum.Versions["node-3"] = shared.StateVersionStamp{Counter: 1}

	eng.RoundOnce(context.Background())

	if got := eng.State.Value; got != 40 {
		t.Fatalf("somma filtrata inattesa: got=%v want=40", got)
	}
	if got := eng.State.AggregationData.Sum.Contributions["node-3"]; got != 50 {
		t.Fatalf("contributo suspect non deve essere cancellato: got=%v", got)
	}
}

func TestRoundAverageFiltraContributiNonEleggibiliSenzaCancellarli(t *testing.T) {
	tr := &captureTransport{}
	m := membership.NewSet()
	m.Upsert(membership.Peer{NodeID: "node-1", Addr: "node-1:7001", Status: membership.Alive, LastSeen: time.Now().UTC()})
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: time.Now().UTC()})
	m.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Dead, LastSeen: time.Now().UTC()})

	eng := NewEngine("node-1", "average", tr, m, slog.Default(), nil, time.Hour, 3)
	eng.State.LocalValue = 10
	eng.State.EnsureAverageMetadata()
	eng.State.AggregationData.Average.Contributions["node-1"] = shared.AverageContribution{Sum: 10, Count: 1}
	eng.State.AggregationData.Average.Contributions["node-2"] = shared.AverageContribution{Sum: 30, Count: 1}
	eng.State.AggregationData.Average.Contributions["node-3"] = shared.AverageContribution{Sum: 90, Count: 1}
	eng.State.AggregationData.Average.Versions["node-1"] = shared.StateVersionStamp{Counter: 1}
	eng.State.AggregationData.Average.Versions["node-2"] = shared.StateVersionStamp{Counter: 1}
	eng.State.AggregationData.Average.Versions["node-3"] = shared.StateVersionStamp{Counter: 1}

	eng.RoundOnce(context.Background())

	if got := eng.State.Value; got != 20 {
		t.Fatalf("media filtrata inattesa: got=%v want=20", got)
	}
	if got := eng.State.AggregationData.Average.Contributions["node-3"]; got != (shared.AverageContribution{Sum: 90, Count: 1}) {
		t.Fatalf("contributo dead non deve essere cancellato: got=%+v", got)
	}
}

func TestRoundMinFiltraContributiNonEleggibiliSenzaCancellarli(t *testing.T) {
	tr := &captureTransport{}
	m := membership.NewSet()
	m.Upsert(membership.Peer{NodeID: "node-1", Addr: "node-1:7001", Status: membership.Alive, LastSeen: time.Now().UTC()})
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: time.Now().UTC()})
	m.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Left, LastSeen: time.Now().UTC()})

	eng := NewEngine("node-1", "min", tr, m, slog.Default(), nil, time.Hour, 3)
	eng.State.LocalValue = 10
	eng.State.EnsureMinMetadata()
	eng.State.AggregationData.Min.Contributions["node-1"] = 10
	eng.State.AggregationData.Min.Contributions["node-2"] = 30
	eng.State.AggregationData.Min.Contributions["node-3"] = 5
	eng.State.AggregationData.Min.Versions["node-1"] = shared.StateVersionStamp{Counter: 1}
	eng.State.AggregationData.Min.Versions["node-2"] = shared.StateVersionStamp{Counter: 1}
	eng.State.AggregationData.Min.Versions["node-3"] = shared.StateVersionStamp{Counter: 1}

	eng.RoundOnce(context.Background())

	if got := eng.State.Value; got != 10 {
		t.Fatalf("minimo filtrato inatteso: got=%v want=10", got)
	}
	if got := eng.State.AggregationData.Min.Contributions["node-3"]; got != 5 {
		t.Fatalf("contributo left non deve essere cancellato: got=%v", got)
	}
}

func TestRoundMaxFiltraContributiNonEleggibiliSenzaCancellarli(t *testing.T) {
	tr := &captureTransport{}
	m := membership.NewSet()
	m.Upsert(membership.Peer{NodeID: "node-1", Addr: "node-1:7001", Status: membership.Alive, LastSeen: time.Now().UTC()})
	m.Upsert(membership.Peer{NodeID: "node-2", Addr: "node-2:7002", Status: membership.Alive, LastSeen: time.Now().UTC()})
	m.Upsert(membership.Peer{NodeID: "node-3", Addr: "node-3:7003", Status: membership.Dead, LastSeen: time.Now().UTC()})

	eng := NewEngine("node-1", "max", tr, m, slog.Default(), nil, time.Hour, 3)
	eng.State.LocalValue = 10
	eng.State.EnsureMaxMetadata()
	eng.State.AggregationData.Max.Contributions["node-1"] = 10
	eng.State.AggregationData.Max.Contributions["node-2"] = 30
	eng.State.AggregationData.Max.Contributions["node-3"] = 90
	eng.State.AggregationData.Max.Versions["node-1"] = shared.StateVersionStamp{Counter: 1}
	eng.State.AggregationData.Max.Versions["node-2"] = shared.StateVersionStamp{Counter: 1}
	eng.State.AggregationData.Max.Versions["node-3"] = shared.StateVersionStamp{Counter: 1}

	eng.RoundOnce(context.Background())

	if got := eng.State.Value; got != 30 {
		t.Fatalf("massimo filtrato inatteso: got=%v want=30", got)
	}
	if got := eng.State.AggregationData.Max.Contributions["node-3"]; got != 90 {
		t.Fatalf("contributo dead non deve essere cancellato: got=%v", got)
	}
}
