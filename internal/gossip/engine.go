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

// CurrentMessageVersion restituisce la versione corrente del contratto messaggio gossip.
func CurrentMessageVersion() shared.MessageVersion {
	return currentMessageVersion
}

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

		membershipEntries := len(msg.Membership)
		incomingEstimate := msg.State.Value
		incomingRound := msg.State.Round

		e.mu.Lock()
		merge := applyRemote(e.State, msg)
		e.State = merge.State
		localRound := e.State.Round
		localEstimate := e.State.Value
		e.mu.Unlock()

		markPeerAlive(e.Membership, e.NodeID, msg.OriginNode, msg.SentAt)
		mergeMembership(e.Membership, msg.Membership)
		if e.Logger != nil {
			logLevel := slog.LevelDebug
			if merge.Status == MergeApplied || merge.Status == MergeConflict {
				logLevel = slog.LevelInfo
			}
			e.Logger.LogAttrs(ctx, logLevel, "merge remoto gossip",
				slog.String("event", "remote_merge"),
				slog.String("node_id", string(e.NodeID)),
				slog.Uint64("round", uint64(localRound)),
				slog.Int("peers", membershipEntries),
				slog.Float64("estimate", localEstimate),
				slog.String("merge_status", string(merge.Status)),
				slog.String("merge_reason", merge.Reason),
				slog.String("remote_node_id", string(msg.OriginNode)),
				slog.Uint64("remote_round", uint64(incomingRound)),
				slog.Float64("remote_estimate", incomingEstimate),
				slog.Int("membership_entries", membershipEntries),
			)
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
	sentAt := time.Now().UTC()
	transitions := e.Membership.ApplyTimeoutTransitions(sentAt)
	e.logMembershipTransitions(ctx, sentAt, transitions)
	membershipSnapshot := e.Membership.Snapshot()
	peers := selectGossipTargets(membershipSnapshot)

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
		e.Logger.Debug("round gossip eseguito",
			"event", "gossip_round",
			"node_id", string(e.NodeID),
			"round", msg.State.Round,
			"peers", len(peers),
			"estimate", msg.State.Value,
			"message_id", msg.MessageID,
			"membership_entries", len(msg.Membership),
		)
	}
}

// markPeerAlive tratta un messaggio gossip valido come heartbeat implicito del nodo origine.
func markPeerAlive(set *membership.Set, selfID, originID shared.NodeID, seenAt time.Time) {
	if set == nil || originID == "" || originID == selfID {
		return
	}
	set.Touch(string(originID), seenAt)
}

// logMembershipTransitions emette un log strutturato per ogni degrado osservato dalla failure detection runtime.
func (e *Engine) logMembershipTransitions(ctx context.Context, now time.Time, transitions []membership.Transition) {
	if e.Logger == nil {
		return
	}
	for _, transition := range transitions {
		e.Logger.Info("transizione membership rilevata",
			"event", "membership_transition",
			"node_id", string(e.NodeID),
			"peer_id", transition.Peer.NodeID,
			"peer_addr", transition.Peer.Addr,
			"previous_status", string(transition.Previous),
			"status", string(transition.Peer.Status),
			"incarnation", transition.Peer.Incarnation,
			"last_seen", transition.Peer.LastSeen.Format(time.RFC3339Nano),
			"elapsed_ms", now.Sub(transition.Peer.LastSeen).Milliseconds(),
		)
	}
}

// selectGossipTargets filtra i peer non raggiungibili per evitare invii inutili.
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

// serializeMembershipDigest converte la membership locale nel digest condiviso via gossip.
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

// mergeMembership applica nel set locale il digest membership ricevuto da remoto.
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

// MergeMembership espone il merge del digest membership per le suite esterne.
func MergeMembership(set *membership.Set, remote []shared.MembershipEntry) {
	mergeMembership(set, remote)
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
	stateCopy := cloneStateForMessage(state)
	stateCopy.SeenMessageIDs = nil
	stateCopy.LastSeenVersionByNode = nil
	return stateCopy
}

// cloneStateForMessage crea una copia profonda della porzione serializzabile dello stato per evitare corse sulle mappe.
func cloneStateForMessage(state shared.GossipState) shared.GossipState {
	clone := state
	clone.AggregationData = cloneAggregationState(state.AggregationData)
	return clone
}

// cloneAggregationState duplica in profondità i metadati specifici dell'aggregazione inclusi nel payload gossip.
func cloneAggregationState(data shared.AggregationState) shared.AggregationState {
	return shared.AggregationState{
		Sum:     cloneSumState(data.Sum),
		Average: cloneAverageState(data.Average),
		Min:     cloneMinState(data.Min),
		Max:     cloneMaxState(data.Max),
	}
}

// cloneSumState duplica contributi e versioni della somma idempotente.
func cloneSumState(sumState *shared.SumState) *shared.SumState {
	if sumState == nil {
		return nil
	}
	clone := &shared.SumState{
		Contributions: make(map[shared.NodeID]float64, len(sumState.Contributions)),
		Versions:      make(map[shared.NodeID]shared.StateVersionStamp, len(sumState.Versions)),
		Overflowed:    sumState.Overflowed,
	}
	for nodeID, contribution := range sumState.Contributions {
		clone.Contributions[nodeID] = contribution
	}
	for nodeID, version := range sumState.Versions {
		clone.Versions[nodeID] = version
	}
	return clone
}

// cloneAverageState duplica contributi e versioni della media convergente.
func cloneAverageState(averageState *shared.AverageState) *shared.AverageState {
	if averageState == nil {
		return nil
	}
	clone := &shared.AverageState{
		Contributions: make(map[shared.NodeID]shared.AverageContribution, len(averageState.Contributions)),
		Versions:      make(map[shared.NodeID]shared.StateVersionStamp, len(averageState.Versions)),
	}
	for nodeID, contribution := range averageState.Contributions {
		clone.Contributions[nodeID] = contribution
	}
	for nodeID, version := range averageState.Versions {
		clone.Versions[nodeID] = version
	}
	return clone
}

// cloneMinState duplica le versioni monotone del minimo.
func cloneMinState(minState *shared.MinState) *shared.MinState {
	if minState == nil {
		return nil
	}
	clone := &shared.MinState{Versions: make(map[shared.NodeID]shared.StateVersionStamp, len(minState.Versions))}
	for nodeID, version := range minState.Versions {
		clone.Versions[nodeID] = version
	}
	return clone
}

// cloneMaxState duplica le versioni monotone del massimo.
func cloneMaxState(maxState *shared.MaxState) *shared.MaxState {
	if maxState == nil {
		return nil
	}
	clone := &shared.MaxState{Versions: make(map[shared.NodeID]shared.StateVersionStamp, len(maxState.Versions))}
	for nodeID, version := range maxState.Versions {
		clone.Versions[nodeID] = version
	}
	return clone
}

// RoundOnce espone un singolo round gossip per i test esterni e interni.
func (e *Engine) RoundOnce(ctx context.Context) {
	e.round(ctx)
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
