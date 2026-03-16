package gossip

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"sdcc-project/internal/membership"
	"sdcc-project/internal/transport"
	shared "sdcc-project/internal/types"
)

var currentMessageVersion = shared.MessageVersion{Major: 1, Minor: 0}

// Engine coordina il ciclo gossip locale.
type Engine struct {
	NodeID      shared.NodeID
	State       shared.GossipState
	Membership  *membership.Set
	Transport   transport.Transport
	Logger      *slog.Logger
	RoundTicker *time.Ticker
	mu          sync.Mutex
}

// NewEngine costruisce un engine con dipendenze minime.
func NewEngine(nodeID, aggregationType string, t transport.Transport, m *membership.Set, logger *slog.Logger, roundEvery time.Duration) *Engine {
	if roundEvery <= 0 {
		roundEvery = time.Second
	}
	return &Engine{
		NodeID: shared.NodeID(nodeID),
		State: shared.GossipState{
			NodeID:          shared.NodeID(nodeID),
			AggregationType: aggregationType,
			UpdatedAt:       time.Now().UTC(),
		},
		Membership:  m,
		Transport:   t,
		Logger:      logger,
		RoundTicker: time.NewTicker(roundEvery),
	}
}

// Start avvia il transport e il loop gossip.
// TODO(tecnico): introdurre fanout random, retry e gestione backpressure.
func (e *Engine) Start(ctx context.Context) error {
	if e.Transport == nil {
		return fmt.Errorf("transport nil")
	}
	if e.Membership == nil {
		return fmt.Errorf("membership nil")
	}

	err := e.Transport.Start(ctx, func(_ context.Context, raw []byte) error {
		var msg shared.GossipMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			return err
		}
		normalizeIncomingMessage(&msg)
		e.mu.Lock()
		merge := applyRemote(e.State, msg)
		e.State = merge.State
		e.mu.Unlock()
		mergeMembership(e.Membership, msg.Membership)
		if e.Logger != nil {
			e.Logger.Debug("merge remoto", "status", merge.Status, "reason", merge.Reason, "from", msg.OriginNode, "message_id", msg.MessageID, "membership_entries", len(msg.Membership))
		}
		return nil
	})
	if err != nil {
		return err
	}

	go e.loop(ctx)
	return nil
}

func (e *Engine) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.RoundTicker.C:
			e.round(ctx)
		}
	}
}

func (e *Engine) round(ctx context.Context) {
	membershipSnapshot := e.Membership.Snapshot()
	peers := selectGossipTargets(membershipSnapshot)
	sentAt := time.Now().UTC()

	e.mu.Lock()
	nextRound := e.State.Round + 1
	nextVersion := e.State.VersionCounter + 1
	e.State.Round = nextRound
	e.State.VersionCounter = nextVersion
	e.State.UpdatedAt = sentAt
	e.State = prepareLocalStateForRound(e.State)

	stateSnapshot := sanitizedStateForMessage(e.State)
	stateVersion := normalizeVersion(stateSnapshot)
	messageID := shared.MessageID(fmt.Sprintf("%s-%d-%d", e.NodeID, nextVersion, sentAt.UnixNano()))
	e.State.LastMessageID = messageID
	e.State.LastSenderNodeID = e.NodeID
	msg := shared.GossipMessage{
		MessageID:    messageID,
		OriginNode:   e.NodeID,
		SentAt:       sentAt,
		Version:      currentMessageVersion,
		StateVersion: stateVersion,
		State:        stateSnapshot,
		Membership:   serializeMembershipDigest(membershipSnapshot),
	}
	e.mu.Unlock()

	raw, _ := json.Marshal(msg)
	for _, p := range peers {
		_ = e.Transport.Send(ctx, p.Addr, raw)
	}

	if e.Logger != nil {
		e.Logger.Debug("gossip round eseguito", "peers", len(peers), "round", msg.State.Round, "membership_entries", len(msg.Membership))
	}
}

func selectGossipTargets(peers []membership.Peer) []membership.Peer {
	out := make([]membership.Peer, 0, len(peers))
	for _, p := range peers {
		if p.Status == membership.Dead || p.Status == membership.Left {
			continue
		}
		out = append(out, p)
	}
	return out
}

func serializeMembershipDigest(peers []membership.Peer) []shared.MembershipEntry {
	entries := make([]shared.MembershipEntry, 0, len(peers))
	for _, p := range peers {
		entries = append(entries, shared.MembershipEntry{
			NodeID:      shared.NodeID(p.NodeID),
			Addr:        p.Addr,
			Status:      string(p.Status),
			Incarnation: p.Incarnation,
			LastSeen:    p.LastSeen,
		})
	}
	return entries
}

func mergeMembership(set *membership.Set, remote []shared.MembershipEntry) {
	if set == nil {
		return
	}
	for _, entry := range remote {
		if entry.NodeID == "" && entry.Addr == "" {
			continue
		}
		st := membership.Status(entry.Status)
		if st == "" {
			st = membership.Alive
		}
		set.Upsert(membership.Peer{
			NodeID:      string(entry.NodeID),
			Addr:        entry.Addr,
			Status:      st,
			Incarnation: entry.Incarnation,
			LastSeen:    entry.LastSeen,
		})
	}
}

func prepareLocalStateForRound(state shared.GossipState) shared.GossipState {
	localVersion := normalizeVersion(state)
	switch state.AggregationType {
	case "sum":
		state.EnsureSumMetadata()
		state.AggregationData.Sum.Versions[state.NodeID] = localVersion
		state.AggregationData.Sum.Contributions[state.NodeID] = state.Value
		state.Value, state.AggregationData.Sum.Overflowed = sumWithSaturation(state.AggregationData.Sum.Contributions, state.AggregationData.Sum.Overflowed)
		return state
	case "average":
		state.EnsureAverageMetadata()
		state.AggregationData.Average.Versions[state.NodeID] = localVersion
		state.AggregationData.Average.Contributions[state.NodeID] = shared.AverageContribution{Sum: state.Value, Count: 1}
		state.Value = averageFromContributions(state.AggregationData.Average.Contributions)
		return state
	default:
		return state
	}
}

func sanitizedStateForMessage(state shared.GossipState) shared.GossipState {
	state.SeenMessageIDs = nil
	state.LastSeenVersionByNode = nil
	return state
}

func normalizeIncomingMessage(msg *shared.GossipMessage) {
	if msg.OriginNode == "" {
		msg.OriginNode = msg.State.NodeID
	}
	if msg.SentAt.IsZero() {
		msg.SentAt = msg.State.UpdatedAt
	}
	if msg.MessageID == "" {
		msg.MessageID = shared.MessageID(fmt.Sprintf("legacy-%s-%d", msg.OriginNode, msg.SentAt.UnixNano()))
	}
	if msg.Version == (shared.MessageVersion{}) {
		msg.Version = currentMessageVersion
	}
	if msg.StateVersion == (shared.StateVersionStamp{}) {
		msg.StateVersion = normalizeVersion(msg.State)
	}
}

// Stop ferma ticker e transport.
func (e *Engine) Stop() error {
	if e.RoundTicker != nil {
		e.RoundTicker.Stop()
	}
	if e.Transport != nil {
		return e.Transport.Close()
	}
	return nil
}
