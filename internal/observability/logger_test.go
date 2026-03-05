package observability

import "testing"

func TestNewLogger(t *testing.T) {
	if l := NewLogger("debug", nil); l == nil {
		t.Fatal("logger nil")
	}
}
