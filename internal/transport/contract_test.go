package transport

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// errTransportClosed rappresenta l'errore contrattuale di invio su transport chiuso.
var errTransportClosed = errors.New("transport chiuso")

// deterministicFakeTransport implementa il contratto Transport senza dipendenze di rete.
//
// Il fake mantiene una queue locale di invii, inoltrando immediatamente il payload al
// MessageHandler registrato in Start. In questo modo i test restano deterministici.
type deterministicFakeTransport struct {
	mu      sync.Mutex
	handler MessageHandler
	closed  bool
	started bool
}

// Start registra il MessageHandler per le consegne locali del fake transport.
func (t *deterministicFakeTransport) Start(_ context.Context, handler MessageHandler) error {
	if handler == nil {
		return errors.New("message handler nil")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return errTransportClosed
	}
	t.handler = handler
	t.started = true
	return nil
}

// Send simula una consegna locale rispettando cancellazione/timeout del context.
func (t *deterministicFakeTransport) Send(ctx context.Context, _ string, payload []byte) error {
	if ctx == nil {
		return errors.New("context nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return errTransportClosed
	}
	handler := t.handler
	t.mu.Unlock()

	if handler != nil {
		if err := handler(ctx, append([]byte(nil), payload...)); err != nil {
			return err
		}
	}
	return nil
}

// Close rende il fake inutilizzabile in modo idempotente.
func (t *deterministicFakeTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func TestTransportContract(t *testing.T) {
	t.Run("delivery", func(t *testing.T) {
		tr := &deterministicFakeTransport{}
		received := make(chan []byte, 1)

		if err := tr.Start(context.Background(), func(_ context.Context, payload []byte) error {
			received <- append([]byte(nil), payload...)
			return nil
		}); err != nil {
			t.Fatalf("start errore: %v", err)
		}

		expected := []byte("payload-deterministico")
		if err := tr.Send(context.Background(), "peer-fake", expected); err != nil {
			t.Fatalf("send errore: %v", err)
		}

		got := <-received
		if string(got) != string(expected) {
			t.Fatalf("payload inatteso: got=%q want=%q", got, expected)
		}
	})

	t.Run("timeout o cancel context", func(t *testing.T) {
		tr := &deterministicFakeTransport{}
		if err := tr.Start(context.Background(), func(context.Context, []byte) error { return nil }); err != nil {
			t.Fatalf("start errore: %v", err)
		}

		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := tr.Send(cancelledCtx, "peer-fake", []byte("x")); !errors.Is(err, context.Canceled) {
			t.Fatalf("atteso context canceled, got=%v", err)
		}
	})

	t.Run("close idempotente", func(t *testing.T) {
		tr := &deterministicFakeTransport{}
		if err := tr.Start(context.Background(), func(context.Context, []byte) error { return nil }); err != nil {
			t.Fatalf("start errore: %v", err)
		}
		if err := tr.Close(); err != nil {
			t.Fatalf("prima close errore: %v", err)
		}
		if err := tr.Close(); err != nil {
			t.Fatalf("seconda close dovrebbe essere idempotente, got=%v", err)
		}
	})

	t.Run("invio dopo close", func(t *testing.T) {
		tr := &deterministicFakeTransport{}
		if err := tr.Start(context.Background(), func(context.Context, []byte) error { return nil }); err != nil {
			t.Fatalf("start errore: %v", err)
		}
		if err := tr.Close(); err != nil {
			t.Fatalf("close errore: %v", err)
		}
		if err := tr.Send(context.Background(), "peer-fake", []byte("x")); !errors.Is(err, errTransportClosed) {
			t.Fatalf("atteso errore send-after-close, got=%v", err)
		}
	})
}
