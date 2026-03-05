package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultValidate(t *testing.T) {
	cfg := Default()
	if err := Validate(cfg); err != nil {
		t.Fatalf("default config non valida: %v", err)
	}
}

func TestLoadYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	content := []byte(`node_id: test-node
bind_address: 0.0.0.0
node_port: 7010
seed_peers: [node-1:7001,node-2:7002]
gossip_interval_ms: 1200
fanout: 3
membership_timeout_ms: 6000
enabled_aggregations: [sum,average]
aggregation: average
log_level: debug
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.NodeID != "test-node" || cfg.Aggregation != "average" || cfg.Fanout != 3 {
		t.Fatalf("config caricata in modo inatteso: %+v", cfg)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("NODE_ID", "env-node")
	t.Setenv("AGGREGATION", "average")
	t.Setenv("ENABLED_AGGREGATIONS", "sum,average")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config con env: %v", err)
	}
	if cfg.NodeID != "env-node" {
		t.Fatalf("override NODE_ID non applicato: %+v", cfg)
	}
	if cfg.Aggregation != "average" {
		t.Fatalf("override AGGREGATION non applicato: %+v", cfg)
	}
}
