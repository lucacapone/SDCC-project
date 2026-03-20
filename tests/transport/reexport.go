package transport

import internaltransport "sdcc-project/internal/transport"

type (
	MessageHandler = internaltransport.MessageHandler
	Transport      = internaltransport.Transport
	NoopTransport  = internaltransport.NoopTransport
	UDPTransport   = internaltransport.UDPTransport
)

var NewUDPTransport = internaltransport.NewUDPTransport
