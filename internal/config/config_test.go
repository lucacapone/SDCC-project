package config

import (
	"os"
	"path/filepath"
	"strings"
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
join_endpoint: bootstrap:9000
bootstrap_peers: [node-4:7004,node-5:7005]
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
	if cfg.JoinEndpoint != "bootstrap:9000" {
		t.Fatalf("join_endpoint inatteso: %+v", cfg)
	}
	if len(cfg.BootstrapPeers) != 2 {
		t.Fatalf("bootstrap_peers inattesi: %+v", cfg.BootstrapPeers)
	}
}

func TestLoadJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	content := []byte(`{
  "node_id": "json-node",
  "bind_address": "127.0.0.1",
  "node_port": 7020,
  "seed_peers": ["node-1:7001"],
  "gossip_interval_ms": 900,
  "fanout": 1,
  "membership_timeout_ms": 5000,
  "enabled_aggregations": ["sum", "average"],
  "aggregation": "sum",
  "log_level": "info"
}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load json config: %v", err)
	}

	if cfg.NodeID != "json-node" || cfg.NodePort != 7020 || cfg.Aggregation != "sum" {
		t.Fatalf("config json inattesa: %+v", cfg)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("NODE_ID", "env-node")
	t.Setenv("AGGREGATION", "average")
	t.Setenv("ENABLED_AGGREGATIONS", "sum,average")
	t.Setenv("JOIN_ENDPOINT", "seed:9010")
	t.Setenv("BOOTSTRAP_PEERS", "node-a:7001,node-b:7002")

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
	if cfg.JoinEndpoint != "seed:9010" {
		t.Fatalf("override JOIN_ENDPOINT non applicato: %+v", cfg)
	}
	if len(cfg.BootstrapPeers) != 2 {
		t.Fatalf("override BOOTSTRAP_PEERS non applicato: %+v", cfg)
	}
}

func TestDiscoveryPeersPreferBootstrapPeers(t *testing.T) {
	cfg := Default()
	cfg.SeedPeers = []string{"seed-1", "seed-2"}
	cfg.BootstrapPeers = []string{"bootstrap-1"}

	got := cfg.DiscoveryPeers()
	if len(got) != 1 || got[0] != "bootstrap-1" {
		t.Fatalf("discovery peers inattesi: %+v", got)
	}
}

func TestValidateFailures(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		errPart string
	}{
		{name: "node_id mancante", mutate: func(c *Config) { c.NodeID = "" }, errPart: "node_id obbligatorio"},
		{name: "porta non valida", mutate: func(c *Config) { c.NodePort = 0 }, errPart: "node_port"},
		{name: "interval non valido", mutate: func(c *Config) { c.GossipIntervalMS = 0 }, errPart: "gossip_interval_ms"},
		{name: "fanout non valido", mutate: func(c *Config) { c.Fanout = 0 }, errPart: "fanout"},
		{name: "timeout non valido", mutate: func(c *Config) { c.MembershipTimeoutMS = 0 }, errPart: "membership_timeout_ms"},
		{name: "aggregazioni vuote", mutate: func(c *Config) { c.EnabledAggregations = nil }, errPart: "enabled_aggregations"},
		{name: "aggregation vuota", mutate: func(c *Config) { c.Aggregation = "" }, errPart: "aggregation obbligatoria"},
		{name: "aggregation non abilitata", mutate: func(c *Config) { c.Aggregation = "median" }, errPart: "non presente"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(&cfg)
			err := Validate(cfg)
			if err == nil {
				t.Fatalf("atteso errore per caso %q", tt.name)
			}
			if !strings.Contains(err.Error(), tt.errPart) {
				t.Fatalf("errore inatteso: %v", err)
			}
		})
	}
}
