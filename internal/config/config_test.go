package config

import "testing"

func TestDefaultValidate(t *testing.T) {
	cfg := Default()
	if err := Validate(cfg); err != nil {
		t.Fatalf("default config non valida: %v", err)
	}
}
