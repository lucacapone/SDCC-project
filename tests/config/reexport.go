package config

import internalconfig "sdcc-project/internal/config"

type Config = internalconfig.Config

var (
	Default  = internalconfig.Default
	Load     = internalconfig.Load
	Validate = internalconfig.Validate
)
