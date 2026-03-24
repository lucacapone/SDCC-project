package membership

import (
	"sync"
	"time"
)

// Status rappresenta lo stato di raggiungibilità di un peer.
type Status string

const (
	Alive   Status = "alive"
	Suspect Status = "suspect"
	Dead    Status = "dead"
	Left    Status = "leave"
)

// Config definisce timeout espliciti per le transizioni di membership.
type Config struct {
	SuspectTimeout time.Duration
	DeadTimeout    time.Duration
	PruneRetention time.Duration
}

var defaultConfig = Config{
	SuspectTimeout: 3 * time.Second,
	DeadTimeout:    6 * time.Second,
	PruneRetention: 18 * time.Second,
}

// Peer rappresenta lo stato locale noto per un nodo remoto.
type Peer struct {
	NodeID      string
	Addr        string
	Status      Status
	Incarnation uint64
	LastSeen    time.Time
}

// Transition descrive una transizione di stato osservata durante la failure detection.
type Transition struct {
	Peer     Peer
	Previous Status
}

// Set mantiene la membership locale thread-safe.
type Set struct {
	mu               sync.RWMutex
	cfg              Config
	selfNodeID       string
	peers            map[string]Peer
	prunedWatermarks map[string]Peer
}

func NewSet() *Set {
	return NewSetWithConfig(defaultConfig)
}

func NewSetWithConfig(cfg Config) *Set {
	if cfg.SuspectTimeout <= 0 {
		cfg.SuspectTimeout = defaultConfig.SuspectTimeout
	}
	if cfg.DeadTimeout <= 0 {
		cfg.DeadTimeout = defaultConfig.DeadTimeout
	}
	if cfg.DeadTimeout <= cfg.SuspectTimeout {
		cfg.DeadTimeout = cfg.SuspectTimeout * 2
	}
	if cfg.PruneRetention <= 0 {
		cfg.PruneRetention = defaultConfig.PruneRetention
	}

	return &Set{cfg: cfg, peers: make(map[string]Peer), prunedWatermarks: make(map[string]Peer)}
}

// SetSelfNodeID registra in modo stabile l'identificativo locale nel set membership.
func (s *Set) SetSelfNodeID(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selfNodeID = nodeID
}

// Join aggiunge un seed peer noto solo tramite endpoint di rete.
//
// Nel bootstrap da configurazione il runtime può conoscere inizialmente solo `host:port`;
// il vero `node_id` logico verrà riallineato non appena il peer remoto propaga la
// propria membership completa via join endpoint o gossip.
func (s *Set) Join(address string, now time.Time) {
	s.Upsert(Peer{NodeID: address, Addr: address, Status: Alive, LastSeen: now})
}

// Upsert applica aggiornamenti deterministici dei peer, rispettando la priorità dell'incarnation.
func (s *Set) Upsert(update Peer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if update.NodeID == "" {
		update.NodeID = update.Addr
	}
	if update.Addr == "" {
		update.Addr = update.NodeID
	}
	if update.Status == "" {
		update.Status = Alive
	}

	watermarkNodeID, watermark, hasWatermark := s.findPrunedWatermarkLocked(update)
	if hasWatermark {
		if update.Incarnation < watermark.Incarnation {
			return
		}
		if update.Incarnation == watermark.Incarnation && rankStatus(update.Status) <= rankStatus(watermark.Status) {
			return
		}
		delete(s.prunedWatermarks, watermarkNodeID)
	}

	resolvedNodeID := update.NodeID
	current, ok := s.peers[resolvedNodeID]
	if !ok && update.Addr != "" {
		if aliasNodeID := s.findNodeIDByAddrLocked(update.Addr); aliasNodeID != "" {
			resolvedNodeID = aliasNodeID
			current = s.peers[aliasNodeID]
			ok = true
		}
	}
	if !ok {
		s.peers[update.NodeID] = update
		return
	}

	if update.Incarnation > current.Incarnation {
		if resolvedNodeID != update.NodeID {
			delete(s.peers, resolvedNodeID)
		}
		s.peers[update.NodeID] = update
		return
	}
	if update.Incarnation < current.Incarnation {
		return
	}

	current.NodeID = update.NodeID
	if update.Addr != "" {
		current.Addr = update.Addr
	}
	if update.LastSeen.After(current.LastSeen) {
		current.LastSeen = update.LastSeen
	}
	current.Status = maxStatus(current.Status, update.Status)
	if resolvedNodeID != update.NodeID {
		delete(s.peers, resolvedNodeID)
	}
	s.peers[update.NodeID] = current
}

// Leave marca un peer come uscito usando il clock di processo.
func (s *Set) Leave(nodeID string) {
	s.LeaveAt(nodeID, time.Now().UTC())
}

// LeaveAt marca un peer come uscito usando un timestamp esplicito.
func (s *Set) LeaveAt(nodeID string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if nodeID == "" {
		return
	}
	peer, ok := s.peers[nodeID]
	if !ok {
		s.peers[nodeID] = Peer{NodeID: nodeID, Addr: nodeID, Status: Left, Incarnation: 1, LastSeen: now}
		return
	}
	peer.Status = Left
	peer.Incarnation++
	peer.LastSeen = now
	s.peers[nodeID] = peer
}

// Touch aggiorna heartbeat di un peer gia' noto senza tentare riconciliazioni canoniche.
func (s *Set) Touch(nodeID string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	peer, ok := s.peers[nodeID]
	if !ok {
		return
	}
	peer.LastSeen = now
	peer.Status = Alive
	s.peers[nodeID] = peer
}

// TouchOrUpsertCanonical aggiorna il peer canonico e promuove eventuali placeholder host:port.
func (s *Set) TouchOrUpsertCanonical(nodeID, addr string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if nodeID == "" {
		return
	}
	if addr == "" {
		addr = nodeID
	}

	resolvedNodeID := nodeID
	peer, ok := s.peers[nodeID]
	if !ok && addr != "" {
		if aliasNodeID := s.findNodeIDByAddrLocked(addr); aliasNodeID != "" {
			resolvedNodeID = aliasNodeID
			peer = s.peers[aliasNodeID]
			ok = true
		}
	}

	if !ok {
		s.peers[nodeID] = Peer{
			NodeID:   nodeID,
			Addr:     addr,
			Status:   Alive,
			LastSeen: now,
		}
		return
	}

	peer.NodeID = nodeID
	if addr != "" {
		peer.Addr = addr
	}
	peer.LastSeen = now
	peer.Status = Alive

	if resolvedNodeID != nodeID {
		delete(s.peers, resolvedNodeID)
	}
	s.peers[nodeID] = peer
}

// Snapshot restituisce copia consistente dei peer correnti.
func (s *Set) Snapshot() []Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Peer, 0, len(s.peers))
	for _, p := range s.peers {
		out = append(out, p)
	}
	return out
}

// ApplyTimeoutTransitions applica transizioni deterministiche Alive -> Suspect -> Dead.
func (s *Set) ApplyTimeoutTransitions(now time.Time) []Transition {
	s.mu.Lock()
	defer s.mu.Unlock()

	updated := make([]Transition, 0)
	for id, p := range s.peers {
		if s.selfNodeID != "" && p.NodeID == s.selfNodeID {
			continue
		}
		if p.Status == Left || p.LastSeen.IsZero() {
			continue
		}
		next := nextStatusForElapsed(p.Status, now.Sub(p.LastSeen), s.cfg)
		if p.Status != next {
			previous := p.Status
			p.Status = next
			s.peers[id] = p
			updated = append(updated, Transition{Peer: p, Previous: previous})
		}
	}
	return updated
}

// Prune rimuove fisicamente peer in stato dead/leave che hanno superato la retention.
//
// La rimozione è deterministica: un peer è eleggibile solo se
// `now - last_seen >= PruneRetention`. Prima della cancellazione viene conservato un
// watermark locale minimale (`node_id`, `addr`, `status`, `incarnation`, `last_seen`)
// che impedisce la reintroduzione di digest obsoleti con incarnation/stato non più recenti.
// Un peer può rientrare solo con un aggiornamento strettamente più nuovo del watermark.
func (s *Set) Prune(now time.Time) []Peer {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.PruneRetention <= 0 {
		return nil
	}

	pruned := make([]Peer, 0)
	for nodeID, peer := range s.peers {
		if peer.Status != Dead && peer.Status != Left {
			continue
		}
		if peer.LastSeen.IsZero() {
			continue
		}
		if now.Sub(peer.LastSeen) < s.cfg.PruneRetention {
			continue
		}
		s.recordPrunedWatermarkLocked(peer)
		delete(s.peers, nodeID)
		pruned = append(pruned, peer)
	}
	return pruned
}

// findNodeIDByAddrLocked risolve l'eventuale placeholder creato dal bootstrap seed-only.
func (s *Set) findNodeIDByAddrLocked(addr string) string {
	for nodeID, peer := range s.peers {
		if peer.Addr == addr {
			return nodeID
		}
	}
	return ""
}

// findPrunedWatermarkLocked cerca un watermark compatibile per node_id o addr del peer aggiornato.
func (s *Set) findPrunedWatermarkLocked(update Peer) (string, Peer, bool) {
	if update.NodeID != "" {
		if watermark, ok := s.prunedWatermarks[update.NodeID]; ok {
			return update.NodeID, watermark, true
		}
	}
	if update.Addr != "" {
		for nodeID, watermark := range s.prunedWatermarks {
			if watermark.Addr == update.Addr {
				return nodeID, watermark, true
			}
		}
	}
	return "", Peer{}, false
}

// recordPrunedWatermarkLocked mantiene il watermark più recente per bloccare reintroduzioni obsolete.
func (s *Set) recordPrunedWatermarkLocked(peer Peer) {
	current, ok := s.prunedWatermarks[peer.NodeID]
	if ok && comparePeerFreshness(peer, current) <= 0 {
		return
	}
	s.prunedWatermarks[peer.NodeID] = peer
}

// comparePeerFreshness ordina peer usando prima incarnation, poi priorità stato e infine last_seen.
func comparePeerFreshness(a, b Peer) int {
	if a.Incarnation != b.Incarnation {
		if a.Incarnation > b.Incarnation {
			return 1
		}
		return -1
	}
	if rankStatus(a.Status) != rankStatus(b.Status) {
		if rankStatus(a.Status) > rankStatus(b.Status) {
			return 1
		}
		return -1
	}
	if a.LastSeen.After(b.LastSeen) {
		return 1
	}
	if a.LastSeen.Before(b.LastSeen) {
		return -1
	}
	return 0
}

func nextStatusForElapsed(current Status, elapsed time.Duration, cfg Config) Status {
	if elapsed > cfg.DeadTimeout {
		return Dead
	}
	if elapsed > cfg.SuspectTimeout {
		if current == Dead || current == Left {
			return current
		}
		return Suspect
	}
	if current == Suspect || current == Dead || current == Left {
		return current
	}
	return Alive
}

func maxStatus(a, b Status) Status {
	if rankStatus(b) > rankStatus(a) {
		return b
	}
	return a
}

func rankStatus(s Status) int {
	switch s {
	case Left:
		return 4
	case Dead:
		return 3
	case Suspect:
		return 2
	default:
		return 1
	}
}
