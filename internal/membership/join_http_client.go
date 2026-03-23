package membership

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const joinPath = "/join"

// HTTPJoinClient implementa JoinClient tramite HTTP+JSON verso il bootstrap endpoint.
//
// L'endpoint configurato mantiene il formato host:porta; il client costruisce in modo
// deterministico l'URL http://<endpoint>/join e applica il context ricevuto alla singola richiesta.
type HTTPJoinClient struct {
	httpClient *http.Client
}

// NewHTTPJoinClient costruisce il client reale di bootstrap con timeout difensivo.
func NewHTTPJoinClient(timeout time.Duration) HTTPJoinClient {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return HTTPJoinClient{httpClient: &http.Client{Timeout: timeout}}
}

// Join invia una JoinRequest al bootstrap remoto e decodifica snapshot/delta membership.
func (c HTTPJoinClient) Join(ctx context.Context, endpoint string, req JoinRequest) (JoinResponse, error) {
	trimmedEndpoint := strings.TrimSpace(endpoint)
	if trimmedEndpoint == "" {
		return JoinResponse{}, ErrJoinNotAvailable
	}

	payload, err := json.Marshal(joinRequestWire{NodeID: req.NodeID, Addr: req.Addr})
	if err != nil {
		return JoinResponse{}, fmt.Errorf("marshal join request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(trimmedEndpoint), bytes.NewReader(payload))
	if err != nil {
		return JoinResponse{}, fmt.Errorf("build join request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return JoinResponse{}, fmt.Errorf("execute join request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return JoinResponse{}, fmt.Errorf("join endpoint ha restituito status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var wire joinResponseWire
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return JoinResponse{}, fmt.Errorf("decode join response: %w", err)
	}

	return JoinResponse{
		Snapshot: fromWirePeers(wire.Snapshot),
		Delta:    fromWirePeers(wire.Delta),
	}, nil
}

func joinURL(endpoint string) string {
	return "http://" + strings.TrimSpace(endpoint) + joinPath
}

type joinRequestWire struct {
	NodeID string `json:"node_id"`
	Addr   string `json:"addr"`
}

type joinResponseWire struct {
	Snapshot []joinPeerWire `json:"snapshot"`
	Delta    []joinPeerWire `json:"delta"`
}

type joinPeerWire struct {
	NodeID      string    `json:"node_id"`
	Addr        string    `json:"addr"`
	Status      Status    `json:"status"`
	Incarnation uint64    `json:"incarnation"`
	LastSeen    time.Time `json:"last_seen"`
}

func fromWirePeers(peers []joinPeerWire) []Peer {
	converted := make([]Peer, 0, len(peers))
	for _, peer := range peers {
		converted = append(converted, Peer{
			NodeID:      peer.NodeID,
			Addr:        peer.Addr,
			Status:      peer.Status,
			Incarnation: peer.Incarnation,
			LastSeen:    peer.LastSeen,
		})
	}
	return converted
}
