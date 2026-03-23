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

	eng := NewEngine("node-1", "average", tr, m, slog.Default(), time.Second)
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
	eng := NewEngine("node-1", "average", tr, m, logger, time.Second)
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

func TestRemoteMergeLoggingRiduceDettagliSensibiliAMetadataUtili(t *testing.T) {
	tr := &spyTransportEngine{}
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mset := membership.NewSet()
	eng := NewEngine("node-1", "sum", tr, mset, logger, time.Hour)

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
