package gossip

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"sdcc-project/internal/membership"
	"sdcc-project/internal/observability"
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
	Collector   *observability.Collector
	RoundTicker *time.Ticker
	mu          sync.Mutex
}

// NewEngine costruisce un engine con dipendenze minime.
func NewEngine(nodeID, aggregationType string, t transport.Transport, m *membership.Set, logger *slog.Logger, collector *observability.Collector, roundEvery time.Duration) *Engine {
	if roundEvery <= 0 {
		roundEvery = time.Second
	}
	if m != nil {
		m.SetSelfNodeID(nodeID)
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
		Collector:   collector,
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

		markPeerAlive(e.Membership, e.NodeID, msg.OriginNode, resolveOriginAddr(ctx, msg), msg.SentAt)
		mergeMembership(e.Membership, string(e.NodeID), collectSelfIdentityAliases(e.Membership, string(e.NodeID)), msg.Membership)
		e.updateObservabilityFromRuntime(localEstimate, string(merge.Status))
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
	e.Membership.Prune(sentAt)
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
		Membership:   serializeMembershipDigest(membershipSnapshot, string(e.NodeID)),
	}
	localEstimate := e.State.Value
	e.mu.Unlock()

	raw, _ := json.Marshal(msg)
	for _, p := range peers {
		_ = e.Transport.Send(ctx, p.Addr, raw)
	}

	e.updateObservabilityAfterRound(localEstimate)
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

// updateObservabilityAfterRound riallinea il collector ai valori runtime dopo un round locale completato.
func (e *Engine) updateObservabilityAfterRound(localEstimate float64) {
	if e.Collector == nil {
		return
	}
	e.Collector.IncTotalRounds()
	e.Collector.SetKnownPeers(len(e.Membership.Snapshot()))
	e.Collector.SetCurrentEstimate(localEstimate)
}

// updateObservabilityFromRuntime aggiorna il collector dopo un merge remoto usando lo stato runtime effettivo.
func (e *Engine) updateObservabilityFromRuntime(localEstimate float64, mergeStatus string) {
	if e.Collector == nil {
		return
	}
	e.Collector.IncRemoteMergeOutcome(mergeStatus)
	e.Collector.SetKnownPeers(len(e.Membership.Snapshot()))
	e.Collector.SetCurrentEstimate(localEstimate)
}

// resolveOriginAddr prova a recuperare l'endpoint reale del nodo origine dal digest ricevuto.
func resolveOriginAddr(ctx context.Context, msg shared.GossipMessage) string {
	for _, entry := range msg.Membership {
		if entry.NodeID == msg.OriginNode && isValidNetworkEndpoint(entry.Addr) {
			return entry.Addr
		}
	}
	if remoteAddr, ok := transport.MessageRemoteAddrFromContext(ctx); ok && isValidNetworkEndpoint(remoteAddr) {
		return remoteAddr
	}
	return ""
}

// markPeerAlive tratta un messaggio gossip valido come heartbeat implicito del nodo origine.
func markPeerAlive(set *membership.Set, selfID, originID shared.NodeID, originAddr string, seenAt time.Time) {
	if set == nil || originID == "" || originID == selfID {
		return
	}

	// Se manca un endpoint affidabile, aggiorniamo solo il peer canonico già noto senza
	// promuovere alias o impostare Addr=node_id.
	if originAddr == "" {
		set.Touch(string(originID), seenAt)
		return
	}

	// Un messaggio valido del nodo origine vale come heartbeat del peer canonico anche
	// quando la membership locale contiene ancora un placeholder seed `host:port`.
	set.TouchOrUpsertCanonical(string(originID), originAddr, seenAt)
}

func isValidNetworkEndpoint(endpoint string) bool {
	if endpoint == "" {
		return false
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return false
	}
	return host != "" && port != ""
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
func serializeMembershipDigest(peers []membership.Peer, selfNodeID string) []shared.MembershipEntry {
	entries := make([]shared.MembershipEntry, 0, len(peers))
	canonicalByAddr := make(map[string]membership.Peer, len(peers))

	// Prima indicizziamo i peer canonici cosi' da poter filtrare gli alias `host:port`
	// quando e' gia' presente la stessa entita' con `node_id` stabile.
	for _, peer := range peers {
		if peer.Addr == "" || peer.NodeID == peer.Addr {
			continue
		}
		canonicalByAddr[peer.Addr] = peer
	}

	for _, peer := range peers {
		if selfNodeID != "" && peer.NodeID == selfNodeID {
			continue
		}
		if canonical, ok := canonicalByAddr[peer.Addr]; ok && peer.NodeID == peer.Addr && canonical.NodeID != peer.NodeID {
			continue
		}
		entries = append(entries, shared.MembershipEntry{
			NodeID:      shared.NodeID(peer.NodeID),
			Addr:        peer.Addr,
			Status:      string(peer.Status),
			Incarnation: peer.Incarnation,
			LastSeen:    peer.LastSeen,
		})
	}
	return entries
}

// mergeMembership applica nel set locale il digest membership ricevuto da remoto.
//
// Il filtro self scarta sia il node_id locale canonico, sia eventuali alias noti
// (ad esempio endpoint advertise `host:port`) presenti in `selfAliases`.
func mergeMembership(set *membership.Set, selfNodeID string, selfAliases map[string]struct{}, remote []shared.MembershipEntry) {
	if set == nil {
		return
	}
	for _, entry := range remote {
		if entry.NodeID == "" && entry.Addr == "" {
			continue
		}
		if isSelfMembershipEntry(entry, selfNodeID, selfAliases) {
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
	mergeMembership(set, "", nil, remote)
}

// MergeMembershipWithSelf espone il merge membership ignorando esplicitamente il nodo locale.
func MergeMembershipWithSelf(set *membership.Set, selfNodeID string, remote []shared.MembershipEntry, selfAliases ...string) {
	mergeMembership(set, selfNodeID, aliasLookup(selfAliases), remote)
}

func isSelfMembershipEntry(entry shared.MembershipEntry, selfNodeID string, selfAliases map[string]struct{}) bool {
	if selfNodeID != "" && strings.EqualFold(strings.TrimSpace(string(entry.NodeID)), strings.TrimSpace(selfNodeID)) {
		return true
	}
	entryNodeIDKey := identityKey(string(entry.NodeID))
	if entryNodeIDKey != "" {
		if _, ok := selfAliases[entryNodeIDKey]; ok {
			return true
		}
	}
	entryAddrKey := identityKey(entry.Addr)
	if entryAddrKey != "" {
		if _, ok := selfAliases[entryAddrKey]; ok {
			return true
		}
	}
	return false
}

func collectSelfIdentityAliases(set *membership.Set, selfNodeID string) map[string]struct{} {
	aliases := make(map[string]struct{})
	selfNodeKey := identityKey(selfNodeID)
	if selfNodeKey != "" {
		aliases[selfNodeKey] = struct{}{}
	}
	if set == nil {
		return aliases
	}

	snapshot := set.Snapshot()
	canonicalAdvertiseAddr := ""
	for _, peer := range snapshot {
		if !strings.EqualFold(strings.TrimSpace(peer.NodeID), strings.TrimSpace(selfNodeID)) {
			continue
		}
		aliases[identityKey(peer.NodeID)] = struct{}{}
		peerAddrKey := identityKey(peer.Addr)
		if peerAddrKey != "" {
			aliases[peerAddrKey] = struct{}{}
			canonicalAdvertiseAddr = peer.Addr
		}
	}
	if canonicalAdvertiseAddr == "" {
		return aliases
	}

	for _, peer := range snapshot {
		if !strings.EqualFold(strings.TrimSpace(peer.Addr), strings.TrimSpace(canonicalAdvertiseAddr)) {
			continue
		}
		if peerKey := identityKey(peer.NodeID); peerKey != "" {
			aliases[peerKey] = struct{}{}
		}
		if peerAddrKey := identityKey(peer.Addr); peerAddrKey != "" {
			aliases[peerAddrKey] = struct{}{}
		}
	}
	return aliases
}

func aliasLookup(selfAliases []string) map[string]struct{} {
	lookup := make(map[string]struct{}, len(selfAliases))
	for _, alias := range selfAliases {
		if aliasKey := identityKey(alias); aliasKey != "" {
			lookup[aliasKey] = struct{}{}
		}
	}
	return lookup
}

func identityKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
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
		// Il contributo locale average deve restare ancorato al valore originario del nodo
		// e non alla stima aggregata corrente, altrimenti i round successivi introducono drift.
		localContribution, hasLocalContribution := state.AggregationData.Average.Contributions[state.NodeID]
		if !hasLocalContribution {
			localContribution = shared.AverageContribution{Sum: state.LocalValue, Count: 1}
			// Manteniamo compatibilita' con il bootstrap legacy dei test/runtime che impostano
			// solo `state.Value`: al primo round usiamo quel valore come seme immutabile locale.
			if state.LocalValue == 0 && state.Value != 0 {
				localContribution = shared.AverageContribution{Sum: state.Value, Count: 1}
				state.LocalValue = state.Value
			}
		}
		state.AggregationData.Average.Versions[state.NodeID] = localVersion
		state.AggregationData.Average.Contributions[state.NodeID] = localContribution
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

// MarkPeerAliveForTest espone il heartbeat implicito per le suite esterne del repository.
func MarkPeerAliveForTest(set *membership.Set, selfID, originID shared.NodeID, originAddr string, seenAt time.Time) {
	markPeerAlive(set, selfID, originID, originAddr, seenAt)
}

// SerializeMembershipDigestForTest espone il filtro del digest membership per le suite esterne.
func SerializeMembershipDigestForTest(peers []membership.Peer) []shared.MembershipEntry {
	return serializeMembershipDigest(peers, "")
}

// SerializeMembershipDigestWithSelfForTest espone il filtro digest con esclusione del nodo locale.
func SerializeMembershipDigestWithSelfForTest(peers []membership.Peer, selfNodeID string) []shared.MembershipEntry {
	return serializeMembershipDigest(peers, selfNodeID)
}
