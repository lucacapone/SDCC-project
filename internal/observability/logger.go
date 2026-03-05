package observability

import (
	"io"
	"log/slog"
	"os"
)

// NewLogger crea un logger strutturato minimale.
// TODO(tecnico): supportare output JSON e correlation-id per round gossip.
func NewLogger(level string, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}

	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slogLevel})
	return slog.New(h)
}
