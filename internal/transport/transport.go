package transport

import "context"

type contextKey string

const messageRemoteAddrContextKey contextKey = "transport_message_remote_addr"

// MessageHandler gestisce payload ricevuti dal layer transport.
type MessageHandler func(context.Context, []byte) error

// Transport definisce il contratto di invio/ricezione tra nodi.
type Transport interface {
	Start(context.Context, MessageHandler) error
	Send(context.Context, string, []byte) error
	Close() error
}

// NoopTransport è uno stub per wiring e test senza rete reale.
type NoopTransport struct{}

func (NoopTransport) Start(context.Context, MessageHandler) error { return nil }
func (NoopTransport) Send(context.Context, string, []byte) error  { return nil }
func (NoopTransport) Close() error                                { return nil }

// WithMessageRemoteAddr arricchisce il contesto handler con l'endpoint sorgente osservato dal transport.
func WithMessageRemoteAddr(ctx context.Context, remoteAddr string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, messageRemoteAddrContextKey, remoteAddr)
}

// MessageRemoteAddrFromContext estrae l'endpoint sorgente inserito dal transport, se presente.
func MessageRemoteAddrFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	raw := ctx.Value(messageRemoteAddrContextKey)
	remoteAddr, ok := raw.(string)
	if !ok || remoteAddr == "" {
		return "", false
	}
	return remoteAddr, true
}
