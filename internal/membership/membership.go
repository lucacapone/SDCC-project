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

// Join aggiunge o aggiorna un peer nella membership.
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

	current, ok := s.peers[update.NodeID]
	if !ok {
		s.peers[update.NodeID] = update
		return
	}

	if update.Incarnation > current.Incarnation {
		s.peers[update.NodeID] = update
		return
	}
	if update.Incarnation < current.Incarnation {
		return
	}

	if update.Addr != "" {
		current.Addr = update.Addr
	}
	if update.LastSeen.After(current.LastSeen) {
		current.LastSeen = update.LastSeen
	}
	current.Status = maxStatus(current.Status, update.Status)
	s.peers[update.NodeID] = current
}

// Leave rimuove un peer dalla membership.
func (s *Set) Leave(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peers, nodeID)
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
func (s *Set) ApplyTimeoutTransitions(now time.Time) []Peer {
	s.mu.Lock()
	defer s.mu.Unlock()

	updated := make([]Peer, 0)
	for id, p := range s.peers {
		next := statusForElapsed(now.Sub(p.LastSeen), s.cfg)
		if p.Status != next {
			p.Status = next
			s.peers[id] = p
			updated = append(updated, p)
		}
	}
	return updated
}

func statusForElapsed(elapsed time.Duration, cfg Config) Status {
	if elapsed > cfg.DeadTimeout {
		return Dead
	}
	if elapsed > cfg.SuspectTimeout {
		return Suspect
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
	case Dead:
		return 3
	case Suspect:
		return 2
	default:
		return 1
	}
}
