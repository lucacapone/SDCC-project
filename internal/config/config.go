package config

import (
	"errors"
	"fmt"
	"os"
)

// Config rappresenta la configurazione runtime del nodo.
type Config struct {
	NodeID       string
	BindAddress  string
	GossipPort   int
	Peers        []string
	RoundEveryMS int
	Fanout       int
	LogLevel     string
	Aggregation  string
}

// Default restituisce una configurazione minima valida per bootstrap locale.
func Default() Config {
	return Config{
		NodeID:       "node-1",
		BindAddress:  "0.0.0.0",
		GossipPort:   7001,
		Peers:        nil,
		RoundEveryMS: 1000,
		Fanout:       2,
		LogLevel:     "info",
		Aggregation:  "sum",
	}
}

// Load carica la configurazione da file.
// TODO(tecnico): implementare parser YAML/JSON e override via env.
func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}

	if _, err := os.ReadFile(path); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	return cfg, errors.New("config parser non ancora implementato")
}

// Validate verifica vincoli minimi della configurazione.
func Validate(cfg Config) error {
	if cfg.NodeID == "" {
		return errors.New("node_id obbligatorio")
	}
	if cfg.GossipPort <= 0 {
		return errors.New("gossip_port deve essere > 0")
	}
	if cfg.RoundEveryMS <= 0 {
		return errors.New("round_interval_ms deve essere > 0")
	}
	if cfg.Fanout <= 0 {
		return errors.New("fanout deve essere > 0")
	}
	if cfg.Aggregation == "" {
		return errors.New("aggregation type obbligatorio")
	}

	return nil
}
