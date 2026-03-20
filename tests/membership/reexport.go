package membership

import internalmembership "sdcc-project/internal/membership"

type (
	Status          = internalmembership.Status
	Config          = internalmembership.Config
	Peer            = internalmembership.Peer
	Set             = internalmembership.Set
	JoinRequest     = internalmembership.JoinRequest
	JoinResponse    = internalmembership.JoinResponse
	JoinClient      = internalmembership.JoinClient
	NoopJoinClient  = internalmembership.NoopJoinClient
	BootstrapResult = internalmembership.BootstrapResult
)

const (
	Alive   = internalmembership.Alive
	Suspect = internalmembership.Suspect
	Dead    = internalmembership.Dead
	Left    = internalmembership.Left
)

var (
	ErrJoinNotAvailable = internalmembership.ErrJoinNotAvailable
	NewSet             = internalmembership.NewSet
	NewSetWithConfig   = internalmembership.NewSetWithConfig
	Bootstrap          = internalmembership.Bootstrap
)
