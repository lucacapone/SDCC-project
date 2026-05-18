package gossip

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"sdcc-project/internal/membership"
	"sdcc-project/internal/observability"
	"sdcc-project/internal/transport"
	shared "sdcc-project/internal/types"
)

var currentMessageVersion = shared.MessageVersion{Major: 1, Minor: 0}

const (
	// metadataOriginAddrKey trasporta esplicitamente l'endpoint canonico del mittente.
	metadataOriginAddrKey = "origin_addr"
)

// CurrentMessageVersion restituisce la versione corrente del contratto messaggio gossip.
func CurrentMessageVersion() shared.MessageVersion {
	return currentMessageVersion
}

// Engine coordina il ciclo gossip locale.
type Engine struct {
	NodeID      shared.NodeID
	State       shared.GossipState
	SelfAddr    string
	Fanout      int
	Membership  *membership.Set
	Transport   transport.Transport
	Logger      *slog.Logger
	Collector   *observability.Collector
	RoundTicker *time.Ticker
	RNG         randomIntn
	mu          sync.Mutex
}

type randomIntn interface {
	Intn(n int) int
}

// NewEngine costruisce un engine con dipendenze minime.
func NewEngine(nodeID, aggregationType string, t transport.Transport, m *membership.Set, logger *slog.Logger, collector *observability.Collector, roundEvery time.Duration, fanout int) *Engine {
	if roundEvery <= 0 {
		roundEvery = time.Second
	}
	if fanout <= 0 {
		fanout = 1
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
		SelfAddr:    resolveSelfAdvertiseAddr(m, nodeID),
		Fanout:      fanout,
		Membership:  m,
		Transport:   t,
		Logger:      logger,
		Collector:   collector,
		RoundTicker: time.NewTicker(roundEvery),
		RNG:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Start avvia il transport e il loop gossip.
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
		membershipSnapshotBefore := e.Membership.Snapshot()

		e.mu.Lock()
		merge := applyRemote(e.State, msg)
		e.State = merge.State
		e.mu.Unlock()

		markPeerAlive(ctx, e.Logger, e.Membership, e.NodeID, msg.OriginNode, resolveOriginAddr(ctx, msg), msg.SentAt)
		mergeMembership(e.Membership, string(e.NodeID), collectSelfIdentityAliases(e.Membership, string(e.NodeID), e.SelfAddr), msg.Membership)
		membershipSnapshot := e.Membership.Snapshot()

		e.mu.Lock()
		estimateAfterAggregationMerge := merge.EstimateAfterAggregationMerge
		e.State = recalculateStateForMembership(e.State, e.NodeID, membershipSnapshot)
		localRound := e.State.Round
		localEstimate := e.State.Value
		averageDetails := averageRemoteMergeDetails(e.State, e.NodeID, membershipSnapshot)
		merge.EstimateAfter = localEstimate
		merge.EstimateAfterMembershipRecalculation = localEstimate
		merge.MembershipRecalculationChanged = estimateChanged(estimateAfterAggregationMerge, localEstimate)
		merge.MembershipEligibilityChanged = membershipEligibilityChanged(e.NodeID, membershipSnapshotBefore, membershipSnapshot)
		merge = classifyRuntimeSideEffects(merge)
		merge.UniqueContributions = countKnownContributions(e.State)
		e.mu.Unlock()

		localPeers := len(membershipSnapshot)
		e.updateObservabilityFromRuntime(localEstimate, localPeers, string(merge.Status))
		if e.Logger != nil {
			nodeDecisionSummary, remoteNodeDecision, nodeConflictID, nodeConflictDecision := summarizeMergeNodeDecisions(merge.NodeDecisions, msg.OriginNode)
			baseAttrs := remoteMergeBaseAttrs(e.NodeID, uint64(localRound), localPeers, localEstimate, merge, msg.OriginNode, averageDetails)
			diagnosticAttrs := remoteMergeDiagnosticAttrs(merge, uint64(incomingRound), incomingEstimate, membershipEntries, nodeDecisionSummary, remoteNodeDecision, nodeConflictID, nodeConflictDecision)

			if remoteMergeNeedsInfoDetails(merge) {
				e.Logger.LogAttrs(ctx, slog.LevelInfo, "merge remoto gossip", appendAttrs(baseAttrs, diagnosticAttrs)...)
			} else if merge.Status == MergeApplied {
				e.Logger.LogAttrs(ctx, slog.LevelInfo, "merge remoto gossip", baseAttrs...)
				e.Logger.LogAttrs(ctx, slog.LevelDebug, "diagnostica merge remoto gossip", appendAttrs(baseAttrs, diagnosticAttrs)...)
			} else {
				e.Logger.LogAttrs(ctx, slog.LevelDebug, "diagnostica merge remoto gossip", appendAttrs(baseAttrs, diagnosticAttrs)...)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	go e.loop(ctx)
	return nil
}

// classifyRuntimeSideEffects distingue gli skip puri dai casi in cui il payload
// aggregativo e' stato ignorato ma la fase runtime successiva ha comunque
// modificato la stima esposta, ad esempio per un ricalcolo membership-aware.
func classifyRuntimeSideEffects(merge MergeResult) MergeResult {
	if merge.Status != MergeSkipped {
		return merge
	}
	if !merge.MembershipRecalculationChanged {
		return merge
	}
	merge.Status = MergePartial
	return merge
}

// averageMergeDetails raccoglie i dettagli che spiegano quali contributi average
// sono stati realmente considerati nella stima membership-aware del nodo locale.
type averageMergeDetails struct {
	enabled                 bool
	knownContributions      int
	eligibleContributions   int
	eligibleContributionIDs []string
	knownContributionIDs    []string
}

// remoteMergeBaseAttrs costruisce il set minimo e stabile di campi INFO per i merge significativi.
func remoteMergeBaseAttrs(nodeID shared.NodeID, round uint64, peers int, estimate float64, merge MergeResult, remoteNodeID shared.NodeID, averageDetails averageMergeDetails) []slog.Attr {
	attrs := []slog.Attr{
		slog.String("event", "remote_merge"),
		slog.String("node_id", string(nodeID)),
		slog.Uint64("round", round),
		slog.Int("peers", peers),
		slog.Float64("estimate", estimate),
		slog.String("merge_status", string(merge.Status)),
		slog.String("remote_node_id", string(remoteNodeID)),
		slog.Bool("aggregation_changed", merge.AggregationChanged),
		slog.Bool("membership_recalculation_changed", merge.MembershipRecalculationChanged),
		slog.Bool("membership_eligibility_changed", merge.MembershipEligibilityChanged),
		slog.Float64("estimate_after_aggregation_merge", merge.EstimateAfterAggregationMerge),
		slog.Float64("estimate_after_membership_recalculation", merge.EstimateAfterMembershipRecalculation),
	}
	if !averageDetails.enabled {
		return attrs
	}
	return append(attrs,
		slog.Int("average_known_contributions", averageDetails.knownContributions),
		slog.Int("average_eligible_contributions", averageDetails.eligibleContributions),
		slog.Any("average_eligible_node_ids", averageDetails.eligibleContributionIDs),
		slog.Any("average_contribution_node_ids", averageDetails.knownContributionIDs),
	)
}

// remoteMergeDiagnosticAttrs isola i dettagli ad alta verbosità, emessi a INFO solo per conflitti/anomalie.
func remoteMergeDiagnosticAttrs(merge MergeResult, remoteRound uint64, remoteEstimate float64, membershipEntries int, nodeDecisionSummary mergeNodeDecisionSummary, remoteNodeDecision string, nodeConflictID string, nodeConflictDecision string) []slog.Attr {
	attrs := []slog.Attr{
		slog.Float64("estimate_before", merge.EstimateBefore),
		slog.Float64("estimate_after", merge.EstimateAfter),
		slog.Uint64("remote_round", remoteRound),
		slog.Float64("remote_estimate", remoteEstimate),
		slog.Int("membership_entries", membershipEntries),
		slog.Int("unique_nodes", merge.UniqueContributions),
		slog.Int("node_decisions_newer_version", nodeDecisionSummary.newerVersion),
		slog.Int("node_decisions_duplicate_ignored", nodeDecisionSummary.duplicateIgnored),
		slog.Int("node_decisions_tie_break", nodeDecisionSummary.tieBreak),
		slog.String("remote_node_decision", remoteNodeDecision),
		slog.Bool("max_preserved", merge.MaxPreserved),
		slog.String("merge_reason", merge.Reason),
	}
	if remoteMergeHasConflictDetails(merge, nodeConflictID) {
		attrs = append(attrs,
			slog.String("conflict_node_id", nodeConflictID),
			slog.String("conflict_decision", nodeConflictDecision),
		)
	}
	return attrs
}

// averageRemoteMergeDetails costruisce una fotografia ordinata dei contributi average
// noti e del sottoinsieme effettivamente usato dal calcolo corrente. La lista
// `average_eligible_node_ids` e' intenzionalmente l'intersezione tra nodi eleggibili
// e contributi noti: coincide quindi con i node_id che entrano nella media esposta.
func averageRemoteMergeDetails(state shared.GossipState, selfID shared.NodeID, membershipSnapshot []membership.Peer) averageMergeDetails {
	if state.AggregationType != "average" || state.AggregationData.Average == nil {
		return averageMergeDetails{}
	}

	contributions := state.AggregationData.Average.Contributions
	eligible := eligibleNodeIDs(selfID, membershipSnapshot)
	knownContributionIDs := sortedContributionNodeIDs(contributions)
	eligibleContributionIDs := make([]string, 0, len(eligible))
	for nodeID := range eligible {
		if _, ok := contributions[nodeID]; ok {
			eligibleContributionIDs = append(eligibleContributionIDs, string(nodeID))
		}
	}
	sort.Strings(eligibleContributionIDs)

	return averageMergeDetails{
		enabled:                 true,
		knownContributions:      len(contributions),
		eligibleContributions:   len(eligibleContributionIDs),
		eligibleContributionIDs: eligibleContributionIDs,
		knownContributionIDs:    knownContributionIDs,
	}
}

// sortedContributionNodeIDs restituisce i node_id dei contributi average in ordine
// stabile, così i log restano confrontabili tra round e test deterministici.
func sortedContributionNodeIDs(contributions map[shared.NodeID]shared.AverageContribution) []string {
	nodeIDs := make([]string, 0, len(contributions))
	for nodeID := range contributions {
		nodeIDs = append(nodeIDs, string(nodeID))
	}
	sort.Strings(nodeIDs)
	return nodeIDs
}

// remoteMergeHasConflictDetails evita campi di conflitto vuoti nei log ordinari e diagnostici.
func remoteMergeHasConflictDetails(merge MergeResult, nodeConflictID string) bool {
	return nodeConflictID != "" || merge.Status == MergeConflict
}

// remoteMergeNeedsInfoDetails abilita i dettagli completi in INFO solo per conflitti o skip anomali.
func remoteMergeNeedsInfoDetails(merge MergeResult) bool {
	if merge.Status == MergeConflict || merge.Status == MergePartial {
		return true
	}
	if merge.Status != MergeSkipped {
		return false
	}
	switch merge.Reason {
	case "self_origin_noop", "duplicate_message_id", "same_version_same_payload", "same_version_semantically_equivalent":
		return false
	default:
		return true
	}
}

// appendAttrs concatena copie indipendenti dei gruppi di attributi per evitare aliasing accidentale.
func appendAttrs(groups ...[]slog.Attr) []slog.Attr {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	attrs := make([]slog.Attr, 0, total)
	for _, group := range groups {
		attrs = append(attrs, group...)
	}
	return attrs
}

type mergeNodeDecisionSummary struct {
	newerVersion     int
	duplicateIgnored int
	tieBreak         int
}

// summarizeMergeNodeDecisions produce una sintesi leggera delle decisioni per nodo
// e isola la decisione relativa al nodo remoto originatore, se disponibile.
func summarizeMergeNodeDecisions(nodeDecisions map[shared.NodeID]string, remoteNodeID shared.NodeID) (mergeNodeDecisionSummary, string, string, string) {
	summary := mergeNodeDecisionSummary{}
	remoteNodeDecision := "not_present"
	nodeConflictID := ""
	nodeConflictDecision := ""

	for nodeID, decision := range nodeDecisions {
		switch decision {
		case "newer_version":
			summary.newerVersion++
		case "duplicate_ignored":
			summary.duplicateIgnored++
		case "tie_break":
			summary.tieBreak++
			if nodeConflictID == "" {
				nodeConflictID = string(nodeID)
				nodeConflictDecision = decision
			}
		}
		if nodeID == remoteNodeID {
			remoteNodeDecision = decision
		}
	}

	return summary, remoteNodeDecision, nodeConflictID, nodeConflictDecision
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
	peers = pickFanoutTargets(peers, e.Fanout, e.RNG)

	e.mu.Lock()
	nextRound := e.State.Round + 1
	nextVersion := e.State.VersionCounter + 1
	e.State.Round = nextRound
	e.State.VersionCounter = nextVersion
	e.State.UpdatedAt = sentAt
	e.State = prepareLocalStateForRound(e.State, e.NodeID, membershipSnapshot)

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
		Metadata:     buildMessageMetadata(string(e.NodeID), membershipSnapshot),
	}
	localEstimate := e.State.Value
	e.mu.Unlock()

	raw, _ := json.Marshal(msg)
	for _, p := range peers {
		_ = e.Transport.Send(ctx, p.Addr, raw)
	}

	e.updateObservabilityAfterRound(localEstimate, len(membershipSnapshot))
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

// AnnounceLeave pubblica un annuncio best-effort di uscita volontaria del nodo locale.
//
// La funzione incrementa l'incarnation del nodo locale in membership, marca lo stato
// `leave` e invia almeno un messaggio gossip ai peer eleggibili prima dello shutdown
// del transport. L'invio è intenzionalmente best-effort: eventuali errori sui singoli
// peer non interrompono il ciclo complessivo di annuncio.
func (e *Engine) AnnounceLeave(ctx context.Context) error {
	if e.Membership == nil {
		return fmt.Errorf("membership nil")
	}

	sentAt := time.Now().UTC()
	e.Membership.LeaveAt(string(e.NodeID), sentAt)
	membershipSnapshot := e.Membership.Snapshot()
	peers := selectGossipTargets(membershipSnapshot)

	e.mu.Lock()
	nextVersion := e.State.VersionCounter + 1
	e.State.Round++
	e.State.VersionCounter = nextVersion
	e.State.UpdatedAt = sentAt
	stateSnapshot := sanitizedStateForMessage(e.State)
	stateVersion := normalizeVersion(stateSnapshot)
	messageID := shared.MessageID(fmt.Sprintf("%s-leave-%d-%d", e.NodeID, nextVersion, sentAt.UnixNano()))
	e.State.LastMessageID = messageID
	e.State.LastSenderNodeID = e.NodeID
	msg := shared.GossipMessage{
		MessageID:    messageID,
		OriginNode:   e.NodeID,
		SentAt:       sentAt,
		Version:      currentMessageVersion,
		StateVersion: stateVersion,
		State:        stateSnapshot,
		// L'annuncio di leave deve includere esplicitamente anche l'entry locale.
		Membership: serializeMembershipDigest(membershipSnapshot, ""),
		Metadata:   buildMessageMetadata(string(e.NodeID), membershipSnapshot),
	}
	e.mu.Unlock()

	raw, _ := json.Marshal(msg)
	for _, peer := range peers {
		_ = e.Transport.Send(ctx, peer.Addr, raw)
	}

	if e.Logger != nil {
		e.Logger.Info("annuncio leave inviato",
			"event", "membership_leave_announcement",
			"node_id", string(e.NodeID),
			"round", msg.State.Round,
			"peers", len(peers),
			"estimate", msg.State.Value,
			"message_id", msg.MessageID,
			"membership_entries", len(msg.Membership),
		)
	}
	return nil
}

// updateObservabilityAfterRound riallinea il collector ai valori runtime dopo un round locale completato.
func (e *Engine) updateObservabilityAfterRound(localEstimate float64, knownPeers int) {
	if e.Collector == nil {
		return
	}
	e.Collector.IncTotalRounds()
	e.Collector.SetKnownPeers(knownPeers)
	e.Collector.SetCurrentEstimate(localEstimate)
}

// updateObservabilityFromRuntime aggiorna il collector dopo un merge remoto usando lo stato runtime effettivo.
func (e *Engine) updateObservabilityFromRuntime(localEstimate float64, knownPeers int, mergeStatus string) {
	if e.Collector == nil {
		return
	}
	e.Collector.IncRemoteMergeOutcome(mergeStatus)
	e.Collector.SetKnownPeers(knownPeers)
	e.Collector.SetCurrentEstimate(localEstimate)
}

// resolveOriginAddr prova a recuperare l'endpoint reale del nodo origine dal digest ricevuto.
func resolveOriginAddr(ctx context.Context, msg shared.GossipMessage) string {
	if metadataAddr := strings.TrimSpace(msg.Metadata[metadataOriginAddrKey]); isValidNetworkEndpoint(metadataAddr) {
		return metadataAddr
	}
	for _, entry := range msg.Membership {
		if entry.NodeID == msg.OriginNode && isValidNetworkEndpoint(entry.Addr) {
			return entry.Addr
		}
	}
	if remoteAddr, ok := transport.MessageRemoteAddrFromContext(ctx); ok && isKnownCanonicalAddr(msg, remoteAddr) {
		return remoteAddr
	}
	return ""
}

// markPeerAlive tratta un messaggio gossip valido come heartbeat implicito del nodo origine.
func markPeerAlive(ctx context.Context, logger *slog.Logger, set *membership.Set, selfID, originID shared.NodeID, originAddr string, seenAt time.Time) {
	if set == nil || originID == "" || originID == selfID {
		return
	}

	debugLogMarkPeerAlive := func(branchReason string, touchedOrPromotedPeer string) {
		if logger == nil || !logger.Enabled(ctx, slog.LevelDebug) {
			return
		}
		logger.Debug("heartbeat implicito gossip processato",
			"event", "gossip_heartbeat_mark_alive",
			"origin_id", string(originID),
			"origin_addr", originAddr,
			"target_peer", touchedOrPromotedPeer,
			"branch_reason", branchReason,
		)
	}

	// Se manca un endpoint affidabile, aggiorniamo solo il peer canonico già noto senza
	// creare nuovi endpoint non validati. Se l'endpoint canonico è già noto localmente,
	// riallineiamo in modo sicuro eventuali alias esistenti sullo stesso addr.
	if originAddr == "" {
		set.Touch(string(originID), seenAt)
		touchOrPromoteKnownAliasesForOrigin(set, string(originID), seenAt)
		debugLogMarkPeerAlive("missing_origin_addr_touch_existing", string(originID))
		return
	}

	// Evitiamo upsert/canonicalizzazione con endpoint non validati: aggiorniamo il peer
	// solo se il canonical addr coincide con quanto il nodo remoto ha dichiarato.
	if isKnownCanonicalOrigin(set, string(originID), originAddr) {
		set.TouchOrUpsertCanonical(string(originID), originAddr, seenAt)
		debugLogMarkPeerAlive("known_canonical_origin_promote_or_touch", string(originID))
		return
	}
	set.Touch(string(originID), seenAt)
	debugLogMarkPeerAlive("unknown_origin_addr_touch_existing_only", string(originID))
}

// touchOrPromoteKnownAliasesForOrigin riallinea alias già presenti in membership verso
// il node_id canonico quando l'endpoint del peer è già noto localmente.
//
// La funzione non introduce mai nuovi endpoint: lavora solo su entry già presenti
// nello snapshot locale e usa TouchOrUpsertCanonical esclusivamente con addr validato.
func touchOrPromoteKnownAliasesForOrigin(set *membership.Set, originID string, seenAt time.Time) {
	if set == nil || originID == "" {
		return
	}
	snapshot := set.Snapshot()
	canonicalAddr := ""
	for _, peer := range snapshot {
		if peer.NodeID == originID && isValidNetworkEndpoint(peer.Addr) {
			canonicalAddr = peer.Addr
			break
		}
	}
	if canonicalAddr == "" {
		return
	}

	knownAliasOnCanonicalAddr := false
	for _, peer := range snapshot {
		if peer.NodeID == originID || peer.Addr != canonicalAddr {
			continue
		}
		knownAliasOnCanonicalAddr = true
		set.Touch(peer.NodeID, seenAt)
	}
	if knownAliasOnCanonicalAddr {
		set.TouchOrUpsertCanonical(originID, canonicalAddr, seenAt)
	}
}

// buildMessageMetadata include metadati minimi e stabili necessari al ricevente.
func buildMessageMetadata(selfNodeID string, peers []membership.Peer) map[string]string {
	originAddr := canonicalAddrByNodeID(peers, selfNodeID)
	if originAddr == "" {
		return nil
	}
	return map[string]string{metadataOriginAddrKey: originAddr}
}

// canonicalAddrByNodeID risolve l'endpoint canonico del nodo cercandolo nello snapshot membership.
func canonicalAddrByNodeID(peers []membership.Peer, nodeID string) string {
	normalizedNodeID := strings.TrimSpace(nodeID)
	for _, peer := range peers {
		if strings.EqualFold(strings.TrimSpace(peer.NodeID), normalizedNodeID) && isValidNetworkEndpoint(peer.Addr) {
			return strings.TrimSpace(peer.Addr)
		}
	}
	return ""
}

// isKnownCanonicalAddr accetta il fallback remoteAddr solo se coincide con endpoint canonicali dichiarati.
func isKnownCanonicalAddr(msg shared.GossipMessage, remoteAddr string) bool {
	trimmed := strings.TrimSpace(remoteAddr)
	if !isValidNetworkEndpoint(trimmed) {
		return false
	}
	for _, entry := range msg.Membership {
		if entry.Addr == trimmed && isValidNetworkEndpoint(entry.Addr) {
			return true
		}
	}
	return false
}

// isKnownCanonicalOrigin verifica che l'endpoint origine corrisponda a un peer già noto localmente.
//
// Casi accettati:
//   - peer già canonico (`node_id == originID` e stesso addr);
//   - peer placeholder bootstrap (`node_id == addr == originAddr`) da promuovere;
//   - (intenzionalmente non più permissivo) nessuna promozione se l'addr non è già conosciuto.
func isKnownCanonicalOrigin(set *membership.Set, originID, originAddr string) bool {
	if set == nil || originID == "" || originAddr == "" {
		return false
	}
	for _, peer := range set.Snapshot() {
		if peer.Addr != originAddr {
			continue
		}
		if peer.NodeID == originID {
			return true
		}
		if peer.NodeID == originAddr {
			return true
		}
		return false
	}
	return false
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

// pickFanoutTargets applica il fanout ai peer eleggibili senza produrre duplicati.
func pickFanoutTargets(peers []membership.Peer, fanout int, rng randomIntn) []membership.Peer {
	if fanout <= 0 {
		fanout = 1
	}
	if fanout >= len(peers) {
		return peers
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	selected := append([]membership.Peer(nil), peers...)
	for i := 0; i < fanout; i++ {
		j := i + rng.Intn(len(selected)-i)
		selected[i], selected[j] = selected[j], selected[i]
	}
	return selected[:fanout]
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
	normalizedSelfNodeID := identityKey(selfNodeID)
	normalizedEntryNodeID := identityKey(string(entry.NodeID))
	normalizedEntryAddr := identityKey(entry.Addr)

	// Manteniamo un confronto esplicito su NodeID con normalizzazione case/trim.
	if normalizedSelfNodeID != "" && normalizedEntryNodeID == normalizedSelfNodeID {
		return true
	}
	// Manteniamo anche il confronto esplicito su Addr con normalizzazione case/trim.
	if normalizedSelfNodeID != "" && normalizedEntryAddr == normalizedSelfNodeID {
		return true
	}
	if normalizedEntryNodeID != "" {
		if _, ok := selfAliases[normalizedEntryNodeID]; ok {
			return true
		}
	}
	if normalizedEntryAddr != "" {
		if _, ok := selfAliases[normalizedEntryAddr]; ok {
			return true
		}
	}
	return false
}

func collectSelfIdentityAliases(set *membership.Set, selfNodeID, selfAdvertiseAddr string) map[string]struct{} {
	aliases := make(map[string]struct{})
	selfNodeKey := identityKey(selfNodeID)
	if selfNodeKey != "" {
		aliases[selfNodeKey] = struct{}{}
	}
	// L'advertise_addr noto deve essere sempre considerato alias locale anche quando
	// il peer self non è ancora presente nello snapshot membership corrente.
	selfAddrKey := identityKey(selfAdvertiseAddr)
	if selfAddrKey != "" {
		aliases[selfAddrKey] = struct{}{}
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

// resolveSelfAdvertiseAddr ricava l'endpoint canonico locale da membership, se già noto.
func resolveSelfAdvertiseAddr(set *membership.Set, selfNodeID string) string {
	if set == nil {
		return ""
	}
	for _, peer := range set.Snapshot() {
		if strings.EqualFold(strings.TrimSpace(peer.NodeID), strings.TrimSpace(selfNodeID)) && isValidNetworkEndpoint(peer.Addr) {
			return strings.TrimSpace(peer.Addr)
		}
	}
	return ""
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

// membershipEligibilityChanged segnala se il digest remoto o l'heartbeat implicito
// hanno modificato il filtro dei node_id eleggibili per il calcolo aggregativo.
func membershipEligibilityChanged(selfID shared.NodeID, before, after []membership.Peer) bool {
	beforeEligible := eligibleNodeIDs(selfID, before)
	afterEligible := eligibleNodeIDs(selfID, after)
	if len(beforeEligible) != len(afterEligible) {
		return true
	}
	for nodeID := range beforeEligible {
		if _, ok := afterEligible[nodeID]; !ok {
			return true
		}
	}
	return false
}

// eligibleNodeIDs costruisce il set dei nodi che possono contribuire al valore
// aggregato corrente secondo la vista membership del round. Sono inclusi solo
// peer Alive; il nodo locale resta incluso quando e' Alive nello snapshot oppure
// quando non e' ancora rappresentato, evitando perdita del contributo self nel
// bootstrap iniziale.
func eligibleNodeIDs(selfID shared.NodeID, peers []membership.Peer) map[shared.NodeID]struct{} {
	eligible := make(map[shared.NodeID]struct{}, len(peers)+1)
	selfSeen := false

	for _, peer := range peers {
		if peer.NodeID == "" {
			continue
		}
		nodeID := shared.NodeID(peer.NodeID)
		if nodeID == selfID {
			selfSeen = true
		}
		// I peer nati dal bootstrap statico usano temporaneamente `host:port`
		// come NodeID. Questi placeholder sono utili per raggiungere il nodo,
		// ma non devono mai diventare chiavi eleggibili per il calcolo
		// aggregativo: i contributi CRDT-like sono indicizzati dal node_id
		// logico canonico propagato dal gossip remoto.
		if !isLogicalAggregationNodeID(nodeID) {
			continue
		}
		if membership.IsAggregationEligible(peer.Status) {
			eligible[nodeID] = struct{}{}
		}
	}

	if selfID != "" && !selfSeen {
		eligible[selfID] = struct{}{}
	}
	return eligible
}

// recalculateStateForMembership riallinea la stima derivata alla membership
// corrente senza cancellare contributi storici. Le mappe per nodo restano quindi
// disponibili per rejoin/recovery, mentre Value rappresenta solo i nodi Alive
// nello snapshot osservato.
func recalculateStateForMembership(state shared.GossipState, selfID shared.NodeID, membershipSnapshot []membership.Peer) shared.GossipState {
	eligible := eligibleNodeIDs(selfID, membershipSnapshot)
	switch state.AggregationType {
	case "sum":
		if state.AggregationData.Sum == nil {
			return state
		}
		state.Value, state.AggregationData.Sum.Overflowed = sumWithSaturationForEligible(state.AggregationData.Sum.Contributions, eligible, state.AggregationData.Sum.Overflowed)
		return state
	case "average":
		if state.AggregationData.Average == nil {
			return state
		}
		state.Value = averageFromEligibleContributions(state.AggregationData.Average.Contributions, eligible)
		return state
	case "min":
		if state.AggregationData.Min == nil {
			return state
		}
		state.Value = minFromEligibleContributions(state.AggregationData.Min.Contributions, eligible)
		return state
	case "max":
		if state.AggregationData.Max == nil {
			return state
		}
		state.Value = maxFromEligibleContributions(state.AggregationData.Max.Contributions, eligible)
		return state
	default:
		return state
	}
}

// countKnownContributions restituisce il numero di contributi CRDT-like noti
// senza applicare filtri di eleggibilita', utile per metriche e log diagnostici.
func countKnownContributions(state shared.GossipState) int {
	switch state.AggregationType {
	case "sum":
		if state.AggregationData.Sum == nil {
			return 0
		}
		return len(state.AggregationData.Sum.Contributions)
	case "average":
		if state.AggregationData.Average == nil {
			return 0
		}
		return len(state.AggregationData.Average.Contributions)
	case "min":
		if state.AggregationData.Min == nil {
			return 0
		}
		return len(state.AggregationData.Min.Contributions)
	case "max":
		if state.AggregationData.Max == nil {
			return 0
		}
		return len(state.AggregationData.Max.Contributions)
	default:
		return 0
	}
}

// sumWithSaturationForEligible calcola la somma solo sui contributi dei nodi
// eleggibili, senza rimuovere metadata storici dalle mappe CRDT-like.
func sumWithSaturationForEligible(contributions map[shared.NodeID]float64, eligible map[shared.NodeID]struct{}, alreadyOverflowed bool) (float64, bool) {
	filtered := make(map[shared.NodeID]float64, len(eligible))
	for nodeID := range eligible {
		contribution, ok := contributions[nodeID]
		if !ok {
			continue
		}
		filtered[nodeID] = contribution
	}
	return sumWithSaturation(filtered, alreadyOverflowed)
}

// averageFromEligibleContributions calcola la media usando solo i contributi
// dei nodi eleggibili, preservando intatte le entry non eleggibili nei metadata.
func averageFromEligibleContributions(contributions map[shared.NodeID]shared.AverageContribution, eligible map[shared.NodeID]struct{}) float64 {
	filtered := make(map[shared.NodeID]shared.AverageContribution, len(eligible))
	for nodeID := range eligible {
		// Difesa aggiuntiva: anche se un chiamante costruisse manualmente una
		// mappa eligible con chiavi `host:port`, la media deve restare basata
		// solo sui node_id logici canonici (`node-1`, `node-2`, ...).
		if !isLogicalAggregationNodeID(nodeID) {
			continue
		}
		contribution, ok := contributions[nodeID]
		if !ok {
			continue
		}
		filtered[nodeID] = contribution
	}
	return averageFromContributions(filtered)
}

// isLogicalAggregationNodeID distingue i node_id logici dagli endpoint di
// bootstrap `host:port`, che possono comparire temporaneamente nella membership
// ma non sono chiavi canoniche per i contributi aggregativi.
func isLogicalAggregationNodeID(nodeID shared.NodeID) bool {
	trimmed := strings.TrimSpace(string(nodeID))
	return trimmed != "" && !isValidNetworkEndpoint(trimmed)
}

// minFromEligibleContributions calcola il minimo solo sui contributi dei nodi
// eleggibili. Se nessun contributore eleggibile ha un contributo noto, ritorna
// 0 in continuita' con sum/average; nei round locali il contributo self viene
// comunque registrato prima del calcolo quando il self e' eleggibile.
func minFromEligibleContributions(contributions map[shared.NodeID]float64, eligible map[shared.NodeID]struct{}) float64 {
	filtered := make(map[shared.NodeID]float64, len(eligible))
	for nodeID := range eligible {
		contribution, ok := contributions[nodeID]
		if !ok {
			continue
		}
		filtered[nodeID] = contribution
	}
	return minFromContributions(filtered)
}

// maxFromEligibleContributions calcola il massimo solo sui contributi dei nodi
// eleggibili. Se nessun contributore eleggibile ha un contributo noto, ritorna
// 0 in continuita' con sum/average; nei round locali il contributo self viene
// comunque registrato prima del calcolo quando il self e' eleggibile.
func maxFromEligibleContributions(contributions map[shared.NodeID]float64, eligible map[shared.NodeID]struct{}) float64 {
	filtered := make(map[shared.NodeID]float64, len(eligible))
	for nodeID := range eligible {
		contribution, ok := contributions[nodeID]
		if !ok {
			continue
		}
		filtered[nodeID] = contribution
	}
	return maxFromContributions(filtered)
}

func prepareLocalStateForRound(state shared.GossipState, selfID shared.NodeID, membershipSnapshot []membership.Peer) shared.GossipState {
	eligible := eligibleNodeIDs(selfID, membershipSnapshot)
	localVersion := normalizeVersion(state)
	switch state.AggregationType {
	case "sum":
		state.EnsureSumMetadata()
		// Manteniamo il contributo locale stabile rispetto al valore originario del nodo.
		// In questo modo `state.Value` resta un dato derivato dalla somma dei contributi
		// e non diventa la sorgente canonica da riserializzare nei round successivi.
		if _, hasLocalContribution := state.AggregationData.Sum.Contributions[state.NodeID]; !hasLocalContribution {
			if state.LocalValue == 0 && state.Value != 0 {
				state.LocalValue = state.Value
			}
		}
		localContribution := state.LocalValue
		if _, hasLocalContribution := state.AggregationData.Sum.Contributions[state.NodeID]; hasLocalContribution {
			localContribution = state.AggregationData.Sum.Contributions[state.NodeID]
		}
		state.AggregationData.Sum.Versions[state.NodeID] = localVersion
		state.AggregationData.Sum.Contributions[state.NodeID] = localContribution
		state.Value, state.AggregationData.Sum.Overflowed = sumWithSaturationForEligible(state.AggregationData.Sum.Contributions, eligible, state.AggregationData.Sum.Overflowed)
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
		state.Value = averageFromEligibleContributions(state.AggregationData.Average.Contributions, eligible)
		return state
	case "min":
		state.EnsureMinMetadata()
		// Il contributo locale min resta stabile e separato dalla stima derivata,
		// cosi' un cambio membership puo' ricalcolare il minimo senza perdere storia.
		localContribution, hasLocalContribution := state.AggregationData.Min.Contributions[state.NodeID]
		if !hasLocalContribution {
			localContribution = localScalarContributionForRound(state)
		}
		state.AggregationData.Min.Versions[state.NodeID] = localVersion
		state.AggregationData.Min.Contributions[state.NodeID] = localContribution
		state.Value = minFromEligibleContributions(state.AggregationData.Min.Contributions, eligible)
		return state
	case "max":
		state.EnsureMaxMetadata()
		// Il contributo locale max resta stabile e separato dalla stima derivata,
		// cosi' un cambio membership puo' ricalcolare il massimo senza perdere storia.
		localContribution, hasLocalContribution := state.AggregationData.Max.Contributions[state.NodeID]
		if !hasLocalContribution {
			localContribution = localScalarContributionForRound(state)
		}
		state.AggregationData.Max.Versions[state.NodeID] = localVersion
		state.AggregationData.Max.Contributions[state.NodeID] = localContribution
		state.Value = maxFromEligibleContributions(state.AggregationData.Max.Contributions, eligible)
		return state
	default:
		return state
	}
}

// localScalarContributionForRound restituisce il contributo stabile del nodo locale
// per aggregazioni scalari min/max, evitando di usare una stima aggregata derivata
// come sorgente nei round successivi.
func localScalarContributionForRound(state shared.GossipState) float64 {
	if state.LocalValue == 0 && state.Value != 0 {
		return state.Value
	}
	return state.LocalValue
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

// cloneMinState duplica contributi e versioni del minimo membership-aware.
func cloneMinState(minState *shared.MinState) *shared.MinState {
	if minState == nil {
		return nil
	}
	clone := &shared.MinState{
		Contributions: make(map[shared.NodeID]float64, len(minState.Contributions)),
		Versions:      make(map[shared.NodeID]shared.StateVersionStamp, len(minState.Versions)),
	}
	for nodeID, contribution := range minState.Contributions {
		clone.Contributions[nodeID] = contribution
	}
	for nodeID, version := range minState.Versions {
		clone.Versions[nodeID] = version
	}
	return clone
}

// cloneMaxState duplica contributi e versioni del massimo membership-aware.
func cloneMaxState(maxState *shared.MaxState) *shared.MaxState {
	if maxState == nil {
		return nil
	}
	clone := &shared.MaxState{
		Contributions: make(map[shared.NodeID]float64, len(maxState.Contributions)),
		Versions:      make(map[shared.NodeID]shared.StateVersionStamp, len(maxState.Versions)),
	}
	for nodeID, contribution := range maxState.Contributions {
		clone.Contributions[nodeID] = contribution
	}
	for nodeID, version := range maxState.Versions {
		clone.Versions[nodeID] = version
	}
	return clone
}

// RoundOnce espone un singolo round gossip per i test esterni e interni.
func (e *Engine) RoundOnce(ctx context.Context) {
	e.round(ctx)
}

// EligibleNodeIDsForTest espone la selezione di eleggibilita' ai test black-box del package esterno.
func EligibleNodeIDsForTest(selfID shared.NodeID, peers []membership.Peer) map[shared.NodeID]struct{} {
	return eligibleNodeIDs(selfID, peers)
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
	markPeerAlive(context.Background(), nil, set, selfID, originID, originAddr, seenAt)
}

// SerializeMembershipDigestForTest espone il filtro del digest membership per le suite esterne.
func SerializeMembershipDigestForTest(peers []membership.Peer) []shared.MembershipEntry {
	return serializeMembershipDigest(peers, "")
}

// SerializeMembershipDigestWithSelfForTest espone il filtro digest con esclusione del nodo locale.
func SerializeMembershipDigestWithSelfForTest(peers []membership.Peer, selfNodeID string) []shared.MembershipEntry {
	return serializeMembershipDigest(peers, selfNodeID)
}

// BuildMessageMetadataForTest espone la costruzione metadata per le suite esterne.
func BuildMessageMetadataForTest(selfNodeID string, peers []membership.Peer) map[string]string {
	return buildMessageMetadata(selfNodeID, peers)
}
