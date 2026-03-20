package transport

import (
	"context"
	"testing"
)

func TestNoopTransportContract(t *testing.T) {
	tr := NoopTransport{}
	if err := tr.Start(context.Background(), func(context.Context, []byte) error { return nil }); err != nil {
		t.Fatalf("start errore: %v", err)
	}
	if err := tr.Send(context.Background(), "peer-1", []byte("x")); err != nil {
		t.Fatalf("send errore: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("close errore: %v", err)
	}
}
