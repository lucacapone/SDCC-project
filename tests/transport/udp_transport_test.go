package transport

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestUDPTransportStartSendClose(t *testing.T) {
	listenAddress := freeUDPAddress(t)
	tr, err := NewUDPTransport(listenAddress)
	if err != nil {
		t.Fatalf("new udp transport: %v", err)
	}

	received := make(chan []byte, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tr.Start(ctx, func(_ context.Context, payload []byte) error {
		received <- append([]byte(nil), payload...)
		return nil
	}); err != nil {
		t.Fatalf("start transport: %v", err)
	}

	message := []byte("payload-gossip")
	sendCtx, sendCancel := context.WithTimeout(context.Background(), time.Second)
	defer sendCancel()
	if err := tr.Send(sendCtx, listenAddress, message); err != nil {
		t.Fatalf("send transport: %v", err)
	}

	select {
	case got := <-received:
		if string(got) != string(message) {
			t.Fatalf("payload ricevuto inatteso: got=%q want=%q", string(got), string(message))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout ricezione payload")
	}

	if err := tr.Close(); err != nil {
		t.Fatalf("close transport: %v", err)
	}
}

func TestUDPTransportSendRespectCancelledContext(t *testing.T) {
	listenAddress := freeUDPAddress(t)
	tr, err := NewUDPTransport(listenAddress)
	if err != nil {
		t.Fatalf("new udp transport: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = tr.Send(ctx, listenAddress, []byte("x"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errore atteso context canceled, got=%v", err)
	}
}

func TestUDPTransportRejectsDoubleStart(t *testing.T) {
	listenAddress := freeUDPAddress(t)
	tr, err := NewUDPTransport(listenAddress)
	if err != nil {
		t.Fatalf("new udp transport: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(_ context.Context, _ []byte) error { return nil }
	if err := tr.Start(ctx, handler); err != nil {
		t.Fatalf("start transport: %v", err)
	}
	defer tr.Close()

	if err := tr.Start(ctx, handler); err == nil {
		t.Fatal("atteso errore su doppio start")
	}
}

func freeUDPAddress(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("alloca porta udp: %v", err)
	}
	defer conn.Close()
	return conn.LocalAddr().String()
}
