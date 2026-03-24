package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const udpReadBufferSize = 64 * 1024

// UDPTransport implementa Transport su UDP mantenendo il contratto a payload grezzo.
//
// Il transport espone solo []byte e indirizzo stringa, lasciando il protocollo applicativo
// completamente all'engine gossip.
type UDPTransport struct {
	listenAddress string

	mu      sync.RWMutex
	conn    net.PacketConn
	closed  bool
	started bool

	closeOnce sync.Once
	done      chan struct{}
	wg        sync.WaitGroup
}

// NewUDPTransport costruisce un adapter UDP concreto pronto per Start.
func NewUDPTransport(listenAddress string) (*UDPTransport, error) {
	if listenAddress == "" {
		return nil, errors.New("listen address obbligatorio")
	}
	if _, err := net.ResolveUDPAddr("udp", listenAddress); err != nil {
		return nil, fmt.Errorf("indirizzo UDP non valido %q: %w", listenAddress, err)
	}
	return &UDPTransport{listenAddress: listenAddress, done: make(chan struct{})}, nil
}

// Start apre la socket UDP e avvia il loop di ricezione.
//
// Il loop termina quando il context viene cancellato o quando viene invocato Close.
func (t *UDPTransport) Start(ctx context.Context, handler MessageHandler) error {
	if handler == nil {
		return errors.New("message handler nil")
	}
	if ctx == nil {
		return errors.New("context nil")
	}

	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return errors.New("transport già chiuso")
	}
	if t.started {
		t.mu.Unlock()
		return errors.New("transport già avviato")
	}

	conn, err := net.ListenPacket("udp", t.listenAddress)
	if err != nil {
		t.mu.Unlock()
		return fmt.Errorf("listen udp %s: %w", t.listenAddress, err)
	}
	t.conn = conn
	t.started = true
	t.mu.Unlock()

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.readLoop(ctx, handler)
	}()

	go func() {
		select {
		case <-ctx.Done():
			_ = t.Close()
		case <-t.done:
		}
	}()

	return nil
}

// Send invia il payload verso l'indirizzo destinazione rispettando timeout/cancellazione del context.
func (t *UDPTransport) Send(ctx context.Context, address string, payload []byte) error {
	if ctx == nil {
		return errors.New("context nil")
	}
	if address == "" {
		return errors.New("address destinazione obbligatorio")
	}

	t.mu.RLock()
	closed := t.closed
	t.mu.RUnlock()
	if closed {
		return errors.New("transport chiuso")
	}

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return fmt.Errorf("dial udp %s: %w", address, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetWriteDeadline(deadline); err != nil {
			return fmt.Errorf("set write deadline: %w", err)
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("write udp verso %s: %w", address, err)
	}
	return nil
}

// Close rilascia le risorse in modo idempotente e attende la chiusura delle goroutine.
func (t *UDPTransport) Close() error {
	var closeErr error
	t.closeOnce.Do(func() {
		t.mu.Lock()
		t.closed = true
		conn := t.conn
		t.conn = nil
		t.mu.Unlock()

		close(t.done)
		if conn != nil {
			closeErr = conn.Close()
		}
	})
	t.wg.Wait()
	return closeErr
}

// readLoop riceve datagrammi UDP e invoca il MessageHandler senza bloccare il lifecycle.
func (t *UDPTransport) readLoop(ctx context.Context, handler MessageHandler) {
	buffer := make([]byte, udpReadBufferSize)
	for {
		if ctx.Err() != nil || t.isClosed() {
			return
		}

		conn := t.currentConn()
		if conn == nil {
			return
		}

		_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		n, remoteAddr, err := conn.ReadFrom(buffer)
		if err != nil {
			if t.isClosed() || ctx.Err() != nil {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			continue
		}

		payload := append([]byte(nil), buffer[:n]...)
		handlerCtx := WithMessageRemoteAddr(ctx, "")
		if remoteAddr != nil {
			handlerCtx = WithMessageRemoteAddr(ctx, remoteAddr.String())
		}
		if err := handler(handlerCtx, payload); err != nil {
			continue
		}
	}
}

// currentConn restituisce una snapshot thread-safe della connessione corrente.
func (t *UDPTransport) currentConn() net.PacketConn {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.conn
}

// isClosed espone in modo thread-safe lo stato di shutdown.
func (t *UDPTransport) isClosed() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.closed
}
