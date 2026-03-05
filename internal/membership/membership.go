package membership

import (
	"sync"
	"time"
)

// Peer rappresenta lo stato locale noto per un nodo remoto.
type Peer struct {
	Address   string
	LastSeen  time.Time
	Suspected bool
}

// Set mantiene la membership locale thread-safe.
type Set struct {
	mu    sync.RWMutex
	peers map[string]Peer
}

func NewSet() *Set {
	return &Set{peers: make(map[string]Peer)}
}

// Join aggiunge o aggiorna un peer nella membership.
func (s *Set) Join(address string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers[address] = Peer{Address: address, LastSeen: now}
}

// Leave rimuove un peer dalla membership.
func (s *Set) Leave(address string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peers, address)
}

// Touch aggiorna heartbeat di un peer.
func (s *Set) Touch(address string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	peer, ok := s.peers[address]
	if !ok {
		return
	}
	peer.LastSeen = now
	peer.Suspected = false
	s.peers[address] = peer
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

// MarkSuspected marca come sospetti i peer oltre timeout.
// TODO(tecnico): introdurre stati Alive/Suspect/Dead e gossip della membership.
func (s *Set) MarkSuspected(now time.Time, timeout time.Duration) []Peer {
	s.mu.Lock()
	defer s.mu.Unlock()

	var suspected []Peer
	for addr, p := range s.peers {
		if now.Sub(p.LastSeen) > timeout {
			p.Suspected = true
			s.peers[addr] = p
			suspected = append(suspected, p)
		}
	}
	return suspected
}
