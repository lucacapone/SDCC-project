package observability

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsExposure(t *testing.T) {
	startedAt := time.Date(2026, time.March, 20, 10, 0, 0, 0, time.UTC)
	collector := NewCollector(startedAt)
	collector.AddRounds(7)
	collector.IncRemoteMergeOutcome("applied")
	collector.IncRemoteMergeOutcome("skipped")
	collector.IncRemoteMergeOutcome("dynamic-peer-should-collapse")
	collector.SetKnownPeers(4)
	collector.SetCurrentEstimate(42.5)
	collector.SetHealthMessage("ok")

	handler := NewMetricsHandler(collector)
	handler.now = func() time.Time {
		return startedAt.Add(12 * time.Second)
	}
	server := httptest.NewServer(handler.Handler())
	defer server.Close()

	metricsResp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("errore richiesta metrics: %v", err)
	}
	defer metricsResp.Body.Close()

	metricsBody, err := io.ReadAll(metricsResp.Body)
	if err != nil {
		t.Fatalf("errore lettura metrics: %v", err)
	}
	metricsText := string(metricsBody)
	for _, expected := range []string{
		"sdcc_node_rounds_total 7",
		"sdcc_node_remote_merges_total{result=\"applied\"} 1",
		"sdcc_node_remote_merges_total{result=\"skipped\"} 1",
		"sdcc_node_remote_merges_total{result=\"unknown\"} 1",
		"sdcc_node_known_peers 4",
		"sdcc_node_estimate 42.5",
		"sdcc_node_uptime_seconds 12.000",
		"sdcc_node_ready 0",
	} {
		if !strings.Contains(metricsText, expected) {
			t.Fatalf("metrica attesa assente %q nel body:\n%s", expected, metricsText)
		}
	}
	if strings.Contains(metricsText, "dynamic-peer-should-collapse") {
		t.Fatalf("label ad alta cardinalità esposta indebitamente: %s", metricsText)
	}

	healthResp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("errore richiesta health: %v", err)
	}
	defer healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("status health inatteso: got=%d want=%d", healthResp.StatusCode, http.StatusOK)
	}

	readyResp, err := http.Get(server.URL + "/ready")
	if err != nil {
		t.Fatalf("errore richiesta ready not-ready: %v", err)
	}
	defer readyResp.Body.Close()
	if readyResp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status ready not-ready inatteso: got=%d want=%d", readyResp.StatusCode, http.StatusServiceUnavailable)
	}

	collector.SetReady(true)
	readyRespAfter, err := http.Get(server.URL + "/ready")
	if err != nil {
		t.Fatalf("errore richiesta ready dopo aggiornamento: %v", err)
	}
	defer readyRespAfter.Body.Close()
	if readyRespAfter.StatusCode != http.StatusOK {
		t.Fatalf("status ready inatteso dopo update: got=%d want=%d", readyRespAfter.StatusCode, http.StatusOK)
	}
}
