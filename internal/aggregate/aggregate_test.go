package aggregate

import "testing"

func TestFactoryPlaceholder(t *testing.T) {
	algo, err := Factory("sum")
	if err != nil {
		t.Fatalf("factory sum errore: %v", err)
	}
	if algo.Type() != "sum" {
		t.Fatalf("tipo inatteso: %s", algo.Type())
	}
}
