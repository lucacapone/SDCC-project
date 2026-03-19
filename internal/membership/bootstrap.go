package membership

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrJoinNotAvailable = errors.New("join endpoint non disponibile")

type JoinRequest struct {
	NodeID string
	Addr   string
}

type JoinResponse struct {
	Snapshot []Peer
	Delta    []Peer
}

type JoinClient interface {
	Join(context.Context, string, JoinRequest) (JoinResponse, error)
}

type NoopJoinClient struct{}

func (NoopJoinClient) Join(context.Context, string, JoinRequest) (JoinResponse, error) {
	return JoinResponse{}, ErrJoinNotAvailable
}

type BootstrapResult struct {
	UsedJoinEndpoint bool
	JoinEndpoint     string
	FallbackUsed     bool
	KnownPeers       int
}

// Bootstrap inizializza la vista membership locale usando prima il join endpoint
// e poi, se necessario, i peer statici di fallback.
func Bootstrap(ctx context.Context, set *Set, req JoinRequest, joinEndpoint string, fallbackPeers []string, client JoinClient, now time.Time) BootstrapResult {
	result := BootstrapResult{JoinEndpoint: joinEndpoint}

	if joinEndpoint != "" && client != nil {
		if res, err := client.Join(ctx, joinEndpoint, req); err == nil {
			result.UsedJoinEndpoint = true
			applyPeers(set, req, now, res.Snapshot)
			applyPeers(set, req, now, res.Delta)
			result.KnownPeers = len(set.Snapshot())
			return result
		}
	}

	result.FallbackUsed = len(fallbackPeers) > 0
	for _, peer := range fallbackPeers {
		if sameEndpoint(peer, req.Addr) || sameNodeID(peer, req.NodeID) {
			continue
		}
		set.Join(peer, now)
	}
	result.KnownPeers = len(set.Snapshot())
	return result
}

func applyPeers(set *Set, self JoinRequest, now time.Time, peers []Peer) {
	for _, peer := range peers {
		if sameNodeID(peer.NodeID, self.NodeID) || sameEndpoint(peer.Addr, self.Addr) {
			continue
		}
		if peer.LastSeen.IsZero() {
			peer.LastSeen = now
		}
		set.Upsert(peer)
	}
}

func sameNodeID(a, b string) bool {
	return strings.TrimSpace(a) != "" && strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func sameEndpoint(a, b string) bool {
	return strings.TrimSpace(a) != "" && strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
