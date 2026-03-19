package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultValidate(t *testing.T) {
	cfg := Default()
	if err := Validate(cfg); err != nil {
		fatalfWithConfig(t, "default config non valida", cfg, err)
	}
}

func TestDefaultEnabledAggregationsIncludeConcretePackages(t *testing.T) {
	cfg := Default()
	expected := map[string]bool{"sum": true, "average": true, "min": true, "max": true}
	for _, kind := range cfg.EnabledAggregations {
		delete(expected, kind)
	}
	if len(expected) != 0 {
		t.Fatalf("enabled_aggregations di default incompleto: mancanti=%v", expected)
	}
}

func TestValidateAcceptsMinAndMaxSemantics(t *testing.T) {
	for _, aggregationKind := range []string{"min", "max"} {
		t.Run(aggregationKind, func(t *testing.T) {
			cfg := Default()
			cfg.EnabledAggregations = []string{"sum", "average", "min", "max"}
			cfg.Aggregation = aggregationKind
			if err := Validate(cfg); err != nil {
				fatalfWithConfig(t, "config con aggregazione valida", cfg, err)
			}
		})
	}
}

// TestValidateConfig resta il punto d'ingresso principale richiesto dal task
// e raccoglie i casi più importanti della validazione con subtest leggibili.
func TestValidateConfig(t *testing.T) {
	// Ogni scenario muta una configurazione valida di base per isolare il motivo
	// dell'errore e verificare che il messaggio rimanga leggibile.
	type testCase struct {
		name        string
		mutate      func(*Config)
		errContains []string
	}

	validBaseConfig := func() Config {
		cfg := Default()
		cfg.BootstrapPeers = []string{"node-1:7001"}
		cfg.SeedPeers = []string{"node-2:7002"}
		return cfg
	}

	cases := []testCase{
		{
			name: "default validi",
			mutate: func(cfg *Config) {
				*cfg = Default()
			},
		},
		{
			name: "parametri obbligatori mancanti node_id",
			mutate: func(cfg *Config) {
				cfg.NodeID = ""
			},
			errContains: []string{"node_id", "obbligatorio"},
		},
		{
			name: "parametri obbligatori mancanti aggregation",
			mutate: func(cfg *Config) {
				cfg.Aggregation = ""
			},
			errContains: []string{"aggregation", "obbligatoria"},
		},
		{
			name: "valori numerici pericolosi gossip_interval_ms zero",
			mutate: func(cfg *Config) {
				cfg.GossipIntervalMS = 0
			},
			errContains: []string{"gossip_interval_ms", "> 0"},
		},
		{
			name: "valori numerici pericolosi fanout zero",
			mutate: func(cfg *Config) {
				cfg.Fanout = 0
			},
			errContains: []string{"fanout", "> 0"},
		},
		{
			name: "valori numerici pericolosi membership_timeout_ms zero",
			mutate: func(cfg *Config) {
				cfg.MembershipTimeoutMS = 0
			},
			errContains: []string{"membership_timeout_ms", "> 0"},
		},
		{
			name: "valori numerici pericolosi node_port fuori range",
			mutate: func(cfg *Config) {
				cfg.NodePort = 70000
			},
			errContains: []string{"node_port", "1 e 65535"},
		},
		{
			name: "aggregazione non supportata tra le abilitate",
			mutate: func(cfg *Config) {
				cfg.EnabledAggregations = []string{"sum", "median"}
			},
			errContains: []string{"enabled_aggregations[1]", "median", "non supportata"},
		},
		{
			name: "aggregazione attiva non supportata",
			mutate: func(cfg *Config) {
				cfg.Aggregation = "median"
			},
			errContains: []string{"aggregation", "median", "non supportata"},
		},
		{
			name: "aggregazione attiva non presente nella whitelist",
			mutate: func(cfg *Config) {
				cfg.EnabledAggregations = []string{"sum"}
				cfg.Aggregation = "average"
			},
			errContains: []string{"aggregation", "average", "enabled_aggregations"},
		},
		{
			name: "errori leggibili su peer list con item vuoto",
			mutate: func(cfg *Config) {
				cfg.BootstrapPeers = []string{"node-1:7001", "   "}
			},
			errContains: []string{"bootstrap_peers", "valore vuoto", "posizione 1"},
		},
		{
			name: "errori leggibili su peer list con duplicato",
			mutate: func(cfg *Config) {
				cfg.SeedPeers = []string{"node-1:7001", "node-1:7001"}
			},
			errContains: []string{"seed_peers", "duplicato inutile", "node-1:7001"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validBaseConfig()
			tc.mutate(&cfg)
			err := Validate(cfg)
			if len(tc.errContains) == 0 {
				if err != nil {
					fatalfWithConfig(t, "config valida rifiutata", cfg, err)
				}
				return
			}
			for _, fragment := range tc.errContains {
				assertErrorContains(t, err, fragment)
			}
		})
	}
}

func TestLoadConfigM06(t *testing.T) {
	t.Run("parsing valido da YAML", func(t *testing.T) {
		path := writeTempConfig(t, "config.yaml", `node_id: test-node
bind_address: 0.0.0.0
advertise_addr: test-node:7010
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

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load yaml config: %v", err)
		}

		assertConfigCore(t, cfg, Config{
			NodeID:              "test-node",
			BindAddress:         "0.0.0.0",
			AdvertiseAddr:       "test-node:7010",
			NodePort:            7010,
			JoinEndpoint:        "bootstrap:9000",
			GossipIntervalMS:    1200,
			Fanout:              3,
			MembershipTimeoutMS: 6000,
			Aggregation:         "average",
			LogLevel:            "debug",
		})
		assertSliceEqual(t, "bootstrap_peers", cfg.BootstrapPeers, []string{"node-4:7004", "node-5:7005"})
		assertSliceEqual(t, "seed_peers", cfg.SeedPeers, []string{"node-1:7001", "node-2:7002"})
		assertSliceEqual(t, "enabled_aggregations", cfg.EnabledAggregations, []string{"sum", "average"})
	})

	t.Run("parsing valido da JSON", func(t *testing.T) {
		path := writeTempConfig(t, "config.json", `{
  "node_id": "json-node",
  "bind_address": "127.0.0.1",
  "advertise_addr": "json-node.internal:7020",
  "node_port": 7020,
  "join_endpoint": "bootstrap:9100",
  "bootstrap_peers": ["node-7:7007"],
  "seed_peers": ["node-1:7001"],
  "gossip_interval_ms": 900,
  "fanout": 1,
  "membership_timeout_ms": 5000,
  "enabled_aggregations": ["sum", "average"],
  "aggregation": "sum",
  "log_level": "info"
}`)

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load json config: %v", err)
		}

		assertConfigCore(t, cfg, Config{
			NodeID:              "json-node",
			BindAddress:         "127.0.0.1",
			AdvertiseAddr:       "json-node.internal:7020",
			NodePort:            7020,
			JoinEndpoint:        "bootstrap:9100",
			GossipIntervalMS:    900,
			Fanout:              1,
			MembershipTimeoutMS: 5000,
			Aggregation:         "sum",
			LogLevel:            "info",
		})
		assertSliceEqual(t, "bootstrap_peers", cfg.BootstrapPeers, []string{"node-7:7007"})
		assertSliceEqual(t, "seed_peers", cfg.SeedPeers, []string{"node-1:7001"})
		assertSliceEqual(t, "enabled_aggregations", cfg.EnabledAggregations, []string{"sum", "average"})
	})

	t.Run("config incompleta applica correttamente i default", func(t *testing.T) {
		path := writeTempConfig(t, "partial.yaml", "node_id: partial-node\nseed_peers: [node-1:7001]\n")

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load partial config: %v", err)
		}

		defaults := Default()
		if cfg.NodeID != "partial-node" {
			t.Fatalf("node_id inatteso: %+v", cfg)
		}
		if cfg.NodePort != defaults.NodePort || cfg.Fanout != defaults.Fanout || cfg.Aggregation != defaults.Aggregation {
			t.Fatalf("default non applicati correttamente: %+v, defaults=%+v", cfg, defaults)
		}
		assertSliceEqual(t, "seed_peers", cfg.SeedPeers, []string{"node-1:7001"})
		assertSliceEqual(t, "enabled_aggregations", cfg.EnabledAggregations, defaults.EnabledAggregations)
	})

	t.Run("override env sovrascrive il file", func(t *testing.T) {
		path := writeTempConfig(t, "env-override.yaml", `node_id: file-node
aggregation: sum
enabled_aggregations: [sum,average]
fanout: 2
node_port: 7100
`)
		t.Setenv("NODE_ID", "env-node")
		t.Setenv("ADVERTISE_ADDR", "env-node.service:7200")
		t.Setenv("AGGREGATION", "average")
		t.Setenv("FANOUT", "5")
		t.Setenv("NODE_PORT", "7200")

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load config con env override: %v", err)
		}
		if cfg.NodeID != "env-node" || cfg.AdvertiseAddr != "env-node.service:7200" || cfg.Aggregation != "average" || cfg.Fanout != 5 || cfg.NodePort != 7200 {
			t.Fatalf("override env non applicato correttamente: %+v", cfg)
		}
	})

	t.Run("default applicati quando il campo non è presente", func(t *testing.T) {
		path := writeTempConfig(t, "missing-fields.json", `{"node_id":"json-partial"}`)

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load missing fields: %v", err)
		}

		defaults := Default()
		if cfg.NodeID != "json-partial" {
			t.Fatalf("node_id inatteso: %+v", cfg)
		}
		if cfg.BindAddress != defaults.BindAddress || cfg.AdvertiseAddr != defaults.AdvertiseAddr || cfg.NodePort != defaults.NodePort || cfg.GossipIntervalMS != defaults.GossipIntervalMS {
			t.Fatalf("default non mantenuti sui campi assenti: %+v, defaults=%+v", cfg, defaults)
		}
	})

	t.Run("mismatch di tipo bloccante in YAML", func(t *testing.T) {
		path := writeTempConfig(t, "invalid-types.yaml", "node_port: abc\nfanout: nope\n")
		_, err := Load(path)
		assertErrorContains(t, err, "node_port")
		assertErrorContains(t, err, "atteso intero")
	})

	t.Run("mismatch di tipo bloccante in JSON", func(t *testing.T) {
		path := writeTempConfig(t, "invalid.json", `{"node_port":"abc","fanout":"nope"}`)
		_, err := Load(path)
		assertErrorContains(t, err, "parse json config")
		assertErrorContains(t, err, "node_port")
	})

	t.Run("edge case peer list malformata", func(t *testing.T) {
		path := writeTempConfig(t, "malformed-peers.yaml", "bootstrap_peers:\n  - node-1:7001\n  - \n")
		_, err := Load(path)
		assertErrorContains(t, err, "lista yaml malformata")
		assertErrorContains(t, err, "bootstrap_peers")
	})

	t.Run("estensione file non supportata", func(t *testing.T) {
		path := writeTempConfig(t, "config.toml", "node_id = 'node-1'\n")
		_, err := Load(path)
		assertErrorContains(t, err, "formato file config non supportato")
	})
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("NODE_ID", "env-node")
	t.Setenv("ADVERTISE_ADDR", "env-node.service:7001")
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
	if cfg.AdvertiseAddr != "env-node.service:7001" {
		t.Fatalf("override ADVERTISE_ADDR non applicato: %+v", cfg)
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

func TestLoadYAMLRejectsInvalidNodePort(t *testing.T) {
	path := writeTempConfig(t, "invalid-node-port.yaml", "node_port: abc\n")

	_, err := Load(path)
	assertErrorContains(t, err, "node_port")
	assertErrorContains(t, err, "atteso intero")
}

func TestLoadYAMLRejectsMalformedPeerList(t *testing.T) {
	path := writeTempConfig(t, "malformed-peers.yaml", "bootstrap_peers:\n  - node-1:7001\n  - \n")

	_, err := Load(path)
	assertErrorContains(t, err, "lista yaml malformata")
	assertErrorContains(t, err, "item vuoto")
}

func TestLoadRejectsUnsupportedExtension(t *testing.T) {
	path := writeTempConfig(t, "config.toml", "node_id = 'node-1'\n")

	_, err := Load(path)
	assertErrorContains(t, err, "formato file config non supportato")
}

func TestLoadJSONRejectsIncompatibleType(t *testing.T) {
	path := writeTempConfig(t, "invalid.json", `{
  "node_port": "abc"
}`)

	_, err := Load(path)
	assertErrorContains(t, err, "parse json config")
	assertErrorContains(t, err, "node_port")
}

func TestValidateRejectsNodePortAboveRange(t *testing.T) {
	cfg := Default()
	cfg.NodePort = 70000

	err := Validate(cfg)
	assertErrorContains(t, err, "node_port deve essere compreso tra 1 e 65535")
}

func TestValidateRejectsPeerWithoutPort(t *testing.T) {
	cfg := Default()
	cfg.BootstrapPeers = []string{"node-1"}

	err := Validate(cfg)
	assertErrorContains(t, err, "bootstrap_peers[0]")
	assertErrorContains(t, err, "atteso formato host:porta valido")
}

func TestValidateRejectsPeerWithNonNumericPort(t *testing.T) {
	cfg := Default()
	cfg.SeedPeers = []string{"node-1:abc"}

	err := Validate(cfg)
	assertErrorContains(t, err, "seed_peers[0]")
	assertErrorContains(t, err, "porta \"abc\" non numerica")
}

func TestValidateRejectsUnsupportedEnabledAggregation(t *testing.T) {
	cfg := Default()
	cfg.EnabledAggregations = []string{"sum", "median"}

	err := Validate(cfg)
	assertErrorContains(t, err, "enabled_aggregations[1]")
	assertErrorContains(t, err, "non supportata")
}

func TestValidateRejectsEmptyAndDuplicateListEntries(t *testing.T) {
	t.Run("bootstrap peer vuoto", func(t *testing.T) {
		cfg := Default()
		cfg.BootstrapPeers = []string{"node-1:7001", "   "}

		err := Validate(cfg)
		assertErrorContains(t, err, "bootstrap_peers contiene un valore vuoto")
	})

	t.Run("seed peer duplicato", func(t *testing.T) {
		cfg := Default()
		cfg.SeedPeers = []string{"node-1:7001", "node-1:7001"}

		err := Validate(cfg)
		assertErrorContains(t, err, "seed_peers contiene un duplicato inutile")
	})

	t.Run("aggregazioni duplicate", func(t *testing.T) {
		cfg := Default()
		cfg.EnabledAggregations = []string{"sum", "sum"}

		err := Validate(cfg)
		assertErrorContains(t, err, "enabled_aggregations contiene un duplicato inutile")
	})
}

func TestAdvertiseEndpointFallsBackToLoopbackWhenBindIsWildcard(t *testing.T) {
	cfg := Default()
	cfg.NodePort = 7015

	if got := cfg.AdvertiseEndpoint(); got != "127.0.0.1:7015" {
		t.Fatalf("advertise endpoint inatteso: got=%s", got)
	}
}

func TestAdvertiseEndpointUsesExplicitAdvertiseAddr(t *testing.T) {
	cfg := Default()
	cfg.BindAddress = "0.0.0.0"
	cfg.AdvertiseAddr = "node1:7001"

	if got := cfg.AdvertiseEndpoint(); got != "node1:7001" {
		t.Fatalf("advertise endpoint esplicito ignorato: got=%s", got)
	}
}

func TestValidateRejectsInvalidAdvertiseAddr(t *testing.T) {
	cfg := Default()
	cfg.AdvertiseAddr = "node1"

	err := Validate(cfg)
	assertErrorContains(t, err, "advertise_addr")
	assertErrorContains(t, err, "atteso formato host:porta valido")
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

// assertConfigCore confronta i campi scalari più importanti letti dal loader.
func assertConfigCore(t *testing.T, got Config, want Config) {
	t.Helper()
	if got.NodeID != want.NodeID ||
		got.BindAddress != want.BindAddress ||
		got.AdvertiseAddr != want.AdvertiseAddr ||
		got.NodePort != want.NodePort ||
		got.JoinEndpoint != want.JoinEndpoint ||
		got.GossipIntervalMS != want.GossipIntervalMS ||
		got.Fanout != want.Fanout ||
		got.MembershipTimeoutMS != want.MembershipTimeoutMS ||
		got.Aggregation != want.Aggregation ||
		got.LogLevel != want.LogLevel {
		t.Fatalf("config scalare inattesa:\n got=%+v\nwant=%+v", got, want)
	}
}

// assertSliceEqual evita confronti rumorosi sulle slice della configurazione.
func assertSliceEqual(t *testing.T, field string, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s inatteso: got=%v want=%v", field, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("%s inatteso in posizione %d: got=%v want=%v", field, index, got, want)
		}
	}
}

// fatalfWithConfig centralizza il messaggio di errore dei test di validazione configurazione.
func fatalfWithConfig(t *testing.T, message string, cfg Config, err error) {
	t.Helper()
	t.Fatalf("%s: %v, cfg=%+v", message, err, cfg)
}

// writeTempConfig scrive un file temporaneo di configurazione per i test di caricamento.
func writeTempConfig(t *testing.T, name string, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

// assertErrorContains verifica che l'errore esista e contenga il frammento atteso.
func assertErrorContains(t *testing.T, err error, expected string) {
	t.Helper()
	if err == nil {
		t.Fatalf("atteso errore contenente %q, ottenuto nil", expected)
	}
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("errore inatteso: %v", err)
	}
}

func TestLoadYAML(t *testing.T) {
	// Test legacy mantenuto come alias leggibile della suite M06 per retrocompatibilità.
	t.Run("alias suite yaml", func(t *testing.T) {
		path := writeTempConfig(t, "legacy-config.yaml", fmt.Sprintf("node_id: %s\nseed_peers: [node-1:7001]\n", "legacy-node"))
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if cfg.NodeID != "legacy-node" {
			t.Fatalf("config caricata in modo inatteso: %+v", cfg)
		}
	})
}

func TestLoadJSON(t *testing.T) {
	// Test legacy minimo mantenuto per non perdere il nome storico della suite.
	t.Run("alias suite json", func(t *testing.T) {
		path := writeTempConfig(t, "legacy-config.json", `{"node_id":"legacy-json-node"}`)
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load json config: %v", err)
		}
		if cfg.NodeID != "legacy-json-node" {
			t.Fatalf("config json inattesa: %+v", cfg)
		}
	})
}
