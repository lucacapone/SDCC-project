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
}

var defaultConfig = Config{
	SuspectTimeout: 3 * time.Second,
	DeadTimeout:    6 * time.Second,
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
	mu    sync.RWMutex
	cfg   Config
	peers map[string]Peer
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

	return &Set{cfg: cfg, peers: make(map[string]Peer)}
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

// Touch aggiorna heartbeat di un peer.
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

// findNodeIDByAddrLocked risolve l'eventuale placeholder creato dal bootstrap seed-only.
func (s *Set) findNodeIDByAddrLocked(addr string) string {
	for nodeID, peer := range s.peers {
		if peer.Addr == addr {
			return nodeID
		}
	}
	return ""
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
