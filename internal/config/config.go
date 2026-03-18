package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config rappresenta la configurazione runtime del nodo.
type Config struct {
	NodeID              string   `json:"node_id"`
	BindAddress         string   `json:"bind_address"`
	NodePort            int      `json:"node_port"`
	JoinEndpoint        string   `json:"join_endpoint"`
	BootstrapPeers      []string `json:"bootstrap_peers"`
	SeedPeers           []string `json:"seed_peers"`
	GossipIntervalMS    int      `json:"gossip_interval_ms"`
	Fanout              int      `json:"fanout"`
	MembershipTimeoutMS int      `json:"membership_timeout_ms"`
	EnabledAggregations []string `json:"enabled_aggregations"`
	Aggregation         string   `json:"aggregation"`
	LogLevel            string   `json:"log_level"`
}

func Default() Config {
	return Config{
		NodeID:              "node-1",
		BindAddress:         "0.0.0.0",
		NodePort:            7001,
		JoinEndpoint:        "",
		BootstrapPeers:      nil,
		SeedPeers:           nil,
		GossipIntervalMS:    1000,
		Fanout:              2,
		MembershipTimeoutMS: 5000,
		EnabledAggregations: []string{"sum", "average", "min", "max"},
		Aggregation:         "sum",
		LogLevel:            "info",
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".json":
			// La precedence resta esplicita: si parte da Default(), poi si applica il file JSON.
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse json config: %w", err)
			}
		case ".yaml", ".yml":
			// La precedence resta esplicita: si parte da Default(), poi si applica il file YAML.
			if err := parseSimpleYAML(raw, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse yaml config: %w", err)
			}
		default:
			return Config{}, fmt.Errorf("formato file config non supportato: %s", filepath.Ext(path))
		}
	}
	// Gli override ambiente vengono applicati solo dopo il file per preservare Default() -> file -> env -> Validate().
	overrideFromEnv(&cfg)
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parseSimpleYAML(raw []byte, cfg *Config) error {
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	currentList := ""
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "- ") || line == "-" {
			if currentList == "" {
				return fmt.Errorf("lista yaml malformata alla riga %d: item fuori da una lista supportata", lineNumber)
			}
			itemText := strings.TrimPrefix(line, "-")
			item := strings.Trim(strings.TrimSpace(itemText), `"'`)
			if item == "" {
				return fmt.Errorf("lista yaml malformata alla riga %d: item vuoto in %s", lineNumber, currentList)
			}
			switch currentList {
			case "bootstrap_peers":
				cfg.BootstrapPeers = append(cfg.BootstrapPeers, item)
			case "seed_peers":
				cfg.SeedPeers = append(cfg.SeedPeers, item)
			case "enabled_aggregations":
				cfg.EnabledAggregations = append(cfg.EnabledAggregations, item)
			default:
				return fmt.Errorf("lista yaml malformata alla riga %d: chiave lista non supportata %q", lineNumber, currentList)
			}
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("yaml non supportato alla riga %d: atteso formato chiave: valore", lineNumber)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		currentList = ""
		if value == "" {
			if key == "bootstrap_peers" || key == "seed_peers" || key == "enabled_aggregations" {
				currentList = key
				// L'assenza di elementi sulla stessa riga è valida solo per liste multilinea supportate.
				clearConfigList(key, cfg)
				continue
			}
			continue
		}

		value = strings.Trim(value, `"'`)
		switch key {
		case "node_id":
			cfg.NodeID = value
		case "bind_address":
			cfg.BindAddress = value
		case "node_port":
			parsed, err := atoiDefault(value, cfg.NodePort)
			if err != nil {
				return fmt.Errorf("campo node_port non valido alla riga %d: %w", lineNumber, err)
			}
			cfg.NodePort = parsed
		case "gossip_interval_ms":
			parsed, err := atoiDefault(value, cfg.GossipIntervalMS)
			if err != nil {
				return fmt.Errorf("campo gossip_interval_ms non valido alla riga %d: %w", lineNumber, err)
			}
			cfg.GossipIntervalMS = parsed
		case "join_endpoint":
			cfg.JoinEndpoint = value
		case "bootstrap_peers":
			parsed, err := parseInlineList(value)
			if err != nil {
				return fmt.Errorf("campo bootstrap_peers non valido alla riga %d: %w", lineNumber, err)
			}
			cfg.BootstrapPeers = parsed
		case "fanout":
			parsed, err := atoiDefault(value, cfg.Fanout)
			if err != nil {
				return fmt.Errorf("campo fanout non valido alla riga %d: %w", lineNumber, err)
			}
			cfg.Fanout = parsed
		case "membership_timeout_ms":
			parsed, err := atoiDefault(value, cfg.MembershipTimeoutMS)
			if err != nil {
				return fmt.Errorf("campo membership_timeout_ms non valido alla riga %d: %w", lineNumber, err)
			}
			cfg.MembershipTimeoutMS = parsed
		case "aggregation":
			cfg.Aggregation = value
		case "log_level":
			cfg.LogLevel = value
		case "seed_peers":
			parsed, err := parseInlineList(value)
			if err != nil {
				return fmt.Errorf("campo seed_peers non valido alla riga %d: %w", lineNumber, err)
			}
			cfg.SeedPeers = parsed
		case "enabled_aggregations":
			parsed, err := parseInlineList(value)
			if err != nil {
				return fmt.Errorf("campo enabled_aggregations non valido alla riga %d: %w", lineNumber, err)
			}
			cfg.EnabledAggregations = parsed
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

// clearConfigList azzera la lista target quando il file dichiara esplicitamente una lista multilinea.
func clearConfigList(key string, cfg *Config) {
	switch key {
	case "bootstrap_peers":
		cfg.BootstrapPeers = nil
	case "seed_peers":
		cfg.SeedPeers = nil
	case "enabled_aggregations":
		cfg.EnabledAggregations = nil
	}
}

func parseInlineList(value string) ([]string, error) {
	trim := strings.TrimSpace(value)
	if !strings.HasPrefix(trim, "[") || !strings.HasSuffix(trim, "]") {
		return nil, fmt.Errorf("atteso formato lista [a,b,c]")
	}
	trim = strings.TrimPrefix(trim, "[")
	trim = strings.TrimSuffix(trim, "]")
	if strings.TrimSpace(trim) == "" {
		return []string{}, nil
	}
	parts := strings.Split(trim, ",")
	out := make([]string, 0, len(parts))
	for index, part := range parts {
		item := strings.Trim(strings.TrimSpace(part), `"'`)
		if item == "" {
			return nil, fmt.Errorf("item vuoto in posizione %d", index)
		}
		out = append(out, item)
	}
	return out, nil
}

// atoiDefault distingue tra valore assente/coperto dal default e valore presente ma non numerico.
func atoiDefault(value string, fallback int) (int, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("atteso intero, ottenuto %q", value)
	}
	return parsed, nil
}

func overrideFromEnv(cfg *Config) {
	overrideString("NODE_ID", &cfg.NodeID)
	overrideString("BIND_ADDRESS", &cfg.BindAddress)
	overrideInt("NODE_PORT", &cfg.NodePort)
	overrideString("JOIN_ENDPOINT", &cfg.JoinEndpoint)
	overrideCSV("BOOTSTRAP_PEERS", &cfg.BootstrapPeers)
	overrideCSV("SEED_PEERS", &cfg.SeedPeers)
	overrideInt("GOSSIP_INTERVAL_MS", &cfg.GossipIntervalMS)
	overrideInt("FANOUT", &cfg.Fanout)
	overrideInt("MEMBERSHIP_TIMEOUT_MS", &cfg.MembershipTimeoutMS)
	overrideCSV("ENABLED_AGGREGATIONS", &cfg.EnabledAggregations)
	overrideString("AGGREGATION", &cfg.Aggregation)
	overrideString("LOG_LEVEL", &cfg.LogLevel)
}

func (c Config) DiscoveryPeers() []string {
	if len(c.BootstrapPeers) > 0 {
		return append([]string(nil), c.BootstrapPeers...)
	}
	return append([]string(nil), c.SeedPeers...)
}

func overrideString(name string, target *string) {
	if value, ok := os.LookupEnv(name); ok && strings.TrimSpace(value) != "" {
		*target = strings.TrimSpace(value)
	}
}

func overrideInt(name string, target *int) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil {
		*target = parsed
	}
}

func overrideCSV(name string, target *[]string) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return
	}
	parsed, err := parseInlineList("[" + value + "]")
	if err == nil {
		*target = parsed
	}
}

func (c Config) MembershipTimeout() time.Duration {
	return time.Duration(c.MembershipTimeoutMS) * time.Millisecond
}

func Validate(cfg Config) error {
	if cfg.NodeID == "" {
		return errors.New("node_id obbligatorio")
	}
	if cfg.NodePort <= 0 {
		return errors.New("node_port deve essere > 0")
	}
	if cfg.GossipIntervalMS <= 0 {
		return errors.New("gossip_interval_ms deve essere > 0")
	}
	if cfg.Fanout <= 0 {
		return errors.New("fanout deve essere > 0")
	}
	if cfg.MembershipTimeoutMS <= 0 {
		return errors.New("membership_timeout_ms deve essere > 0")
	}
	if len(cfg.EnabledAggregations) == 0 {
		return errors.New("enabled_aggregations deve contenere almeno un valore")
	}
	if cfg.Aggregation == "" {
		return errors.New("aggregation obbligatoria")
	}
	if !contains(cfg.EnabledAggregations, cfg.Aggregation) {
		return fmt.Errorf("aggregation %q non presente in enabled_aggregations", cfg.Aggregation)
	}
	return nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
