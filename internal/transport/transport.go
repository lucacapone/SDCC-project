package transport

import "context"

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
