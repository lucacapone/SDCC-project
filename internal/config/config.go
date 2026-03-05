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
		SeedPeers:           nil,
		GossipIntervalMS:    1000,
		Fanout:              2,
		MembershipTimeoutMS: 5000,
		EnabledAggregations: []string{"sum", "average"},
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
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse json config: %w", err)
			}
		case ".yaml", ".yml":
			if err := parseSimpleYAML(raw, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse yaml config: %w", err)
			}
		default:
			return Config{}, fmt.Errorf("estensione config non supportata: %s", filepath.Ext(path))
		}
	}
	overrideFromEnv(&cfg)
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parseSimpleYAML(raw []byte, cfg *Config) error {
	s := bufio.NewScanner(strings.NewReader(string(raw)))
	currentList := ""
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			switch currentList {
			case "seed_peers":
				cfg.SeedPeers = append(cfg.SeedPeers, item)
			case "enabled_aggregations":
				cfg.EnabledAggregations = append(cfg.EnabledAggregations, item)
			}
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		currentList = ""
		if value == "" {
			if key == "seed_peers" || key == "enabled_aggregations" {
				currentList = key
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
			cfg.NodePort = atoiDefault(value, cfg.NodePort)
		case "gossip_interval_ms":
			cfg.GossipIntervalMS = atoiDefault(value, cfg.GossipIntervalMS)
		case "fanout":
			cfg.Fanout = atoiDefault(value, cfg.Fanout)
		case "membership_timeout_ms":
			cfg.MembershipTimeoutMS = atoiDefault(value, cfg.MembershipTimeoutMS)
		case "aggregation":
			cfg.Aggregation = value
		case "log_level":
			cfg.LogLevel = value
		case "seed_peers":
			cfg.SeedPeers = parseInlineList(value)
		case "enabled_aggregations":
			cfg.EnabledAggregations = parseInlineList(value)
		}
	}
	if err := s.Err(); err != nil {
		return err
	}
	return nil
}

func parseInlineList(value string) []string {
	trim := strings.TrimSpace(value)
	trim = strings.TrimPrefix(trim, "[")
	trim = strings.TrimSuffix(trim, "]")
	parts := strings.Split(trim, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		item := strings.Trim(strings.TrimSpace(p), `"'`)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func atoiDefault(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func overrideFromEnv(cfg *Config) {
	overrideString("NODE_ID", &cfg.NodeID)
	overrideString("BIND_ADDRESS", &cfg.BindAddress)
	overrideInt("NODE_PORT", &cfg.NodePort)
	overrideCSV("SEED_PEERS", &cfg.SeedPeers)
	overrideInt("GOSSIP_INTERVAL_MS", &cfg.GossipIntervalMS)
	overrideInt("FANOUT", &cfg.Fanout)
	overrideInt("MEMBERSHIP_TIMEOUT_MS", &cfg.MembershipTimeoutMS)
	overrideCSV("ENABLED_AGGREGATIONS", &cfg.EnabledAggregations)
	overrideString("AGGREGATION", &cfg.Aggregation)
	overrideString("LOG_LEVEL", &cfg.LogLevel)
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
	*target = parseInlineList(value)
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
