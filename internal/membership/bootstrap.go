package membership

import (
	"context"
	"errors"
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

// Bootstrap inizializza la vista membership locale usando discovery seed-only.
// Il join endpoint è usato esclusivamente per ottenere peer iniziali, senza ruolo autoritativo.
func Bootstrap(ctx context.Context, set *Set, req JoinRequest, joinEndpoint string, fallbackPeers []string, client JoinClient, now time.Time) BootstrapResult {
	result := BootstrapResult{JoinEndpoint: joinEndpoint}

	if joinEndpoint != "" && client != nil {
		if res, err := client.Join(ctx, joinEndpoint, req); err == nil {
			result.UsedJoinEndpoint = true
			applyPeers(set, req.NodeID, now, res.Snapshot)
			applyPeers(set, req.NodeID, now, res.Delta)
			result.KnownPeers = len(set.Snapshot())
			return result
		}
	}

	result.FallbackUsed = len(fallbackPeers) > 0
	for _, peer := range fallbackPeers {
		if peer == "" || peer == req.NodeID || peer == req.Addr {
			continue
		}
		set.Join(peer, now)
	}
	result.KnownPeers = len(set.Snapshot())
	return result
}

func applyPeers(set *Set, selfNodeID string, now time.Time, peers []Peer) {
	for _, peer := range peers {
		if peer.NodeID == selfNodeID || peer.Addr == selfNodeID {
			continue
		}
		if peer.LastSeen.IsZero() {
			peer.LastSeen = now
		}
		set.Upsert(peer)
	}
}
