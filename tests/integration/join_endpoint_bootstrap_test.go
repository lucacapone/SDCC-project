package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	joinBootstrapGossipTimeout  = 6 * time.Second
	joinBootstrapGossipInterval = 25
)

type observedJoinRequest struct {
	NodeID string `json:"node_id"`
	Addr   string `json:"addr"`
}

type joinResponsePayload struct {
	Snapshot []joinPeerPayload `json:"snapshot"`
	Delta    []joinPeerPayload `json:"delta"`
}

type joinPeerPayload struct {
	NodeID      string    `json:"node_id"`
	Addr        string    `json:"addr"`
	Status      string    `json:"status"`
	Incarnation uint64    `json:"incarnation"`
	LastSeen    time.Time `json:"last_seen"`
}

func TestNodeBootstrapViaJoinEndpointPopulatesInitialMembership(t *testing.T) {
	peerListener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp peer: %v", err)
	}
	defer peerListener.Close()

	joinRequests := make(chan observedJoinRequest, 1)
	base := time.Now().UTC()
	joinServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/join" {
			http.NotFound(w, r)
			return
		}

		defer r.Body.Close()
		var req observedJoinRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		select {
		case joinRequests <- req:
		default:
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(joinResponsePayload{
			Snapshot: []joinPeerPayload{{
				NodeID:      "node-seed",
				Addr:        peerListener.LocalAddr().String(),
				Status:      "alive",
				Incarnation: 1,
				LastSeen:    base,
			}},
		})
	}))
	defer joinServer.Close()

	configPath := writeJoinBootstrapConfig(t, joinServer.Listener.Addr().String())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repoRoot := joinBootstrapRepoRoot(t)
	binaryPath := buildNodeBinary(t)
	cmd := exec.CommandContext(ctx, binaryPath, "--config", configPath)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "OBSERVABILITY_ADDR=127.0.0.1:0")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		t.Fatalf("start node process: %v", err)
	}

	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		waitDone := make(chan error, 1)
		go func() { waitDone <- cmd.Wait() }()
		select {
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-waitDone
		case <-waitDone:
		}
	}()

	select {
	case req := <-joinRequests:
		if req.NodeID != "node-join-test" {
			t.Fatalf("node_id join inatteso: %+v", req)
		}
		if req.Addr == "" {
			t.Fatalf("addr join vuoto: %+v", req)
		}
	case <-time.After(joinBootstrapGossipTimeout):
		t.Fatalf("join request non osservata; output:\n%s", output.String())
	}

	if err := peerListener.SetReadDeadline(time.Now().Add(joinBootstrapGossipTimeout)); err != nil {
		t.Fatalf("set deadline udp: %v", err)
	}
	buffer := make([]byte, 64*1024)
	n, _, err := peerListener.ReadFrom(buffer)
	if err != nil {
		t.Fatalf("nessun gossip verso il peer bootstrap restituito dal join endpoint: %v\noutput:\n%s", err, output.String())
	}
	if n == 0 {
		t.Fatalf("payload gossip vuoto ricevuto dal peer bootstrap")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), "\"used_join_endpoint\":true") || strings.Contains(output.String(), "used_join_endpoint=true") {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("il runtime non ha riportato l'uso del join endpoint reale; output:\n%s", output.String())
}

func writeJoinBootstrapConfig(t *testing.T, joinEndpoint string) string {
	t.Helper()

	nodePort := reserveUDPPort(t)
	tempDir := t.TempDir()
	cfg := fmt.Sprintf(`node_id: node-join-test
bind_address: 127.0.0.1
node_port: %d
advertise_addr: 127.0.0.1:%d
join_endpoint: %s
gossip_interval_ms: %d
fanout: 1
membership_timeout_ms: 5000
enabled_aggregations: [sum,average,min,max]
aggregation: sum
log_level: info
`, nodePort, nodePort, joinEndpoint, joinBootstrapGossipInterval)
	path := filepath.Join(tempDir, "join-bootstrap.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func joinBootstrapRepoRoot(t *testing.T) string {
	t.Helper()
	return integrationRepoRoot(t)
}

func buildNodeBinary(t *testing.T) string {
	t.Helper()

	binaryPath := filepath.Join(t.TempDir(), "sdcc-node-test")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/node")
	buildCmd.Dir = joinBootstrapRepoRoot(t)
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build node binary: %v\noutput:\n%s", err, string(output))
	}
	return binaryPath
}

func reserveUDPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve udp port: %v", err)
	}
	defer listener.Close()
	addr, ok := listener.LocalAddr().(*net.UDPAddr)
	if !ok {
		t.Fatalf("local addr non UDP: %T", listener.LocalAddr())
	}
	return addr.Port
}
