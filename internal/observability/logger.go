package observability

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// LoggerOptions raccoglie le opzioni minime per costruire un logger coerente con slog.
type LoggerOptions struct {
	// Level definisce il livello minimo abilitato (debug, info, warn, error).
	Level string
	// Writer definisce la destinazione di output; se nil viene usato stdout.
	Writer io.Writer
}

// NewLogger crea un logger strutturato minimale e coerente con log/slog.
//
// Il logger usa volutamente un handler testuale standard per mantenere dipendenze
// minime e produrre log già strutturati come coppie chiave/valore.
func NewLogger(level string, w io.Writer) *slog.Logger {
	return NewLoggerWithOptions(LoggerOptions{Level: level, Writer: w})
}

// NewLoggerWithOptions crea un logger strutturato usando opzioni esplicite.
func NewLoggerWithOptions(options LoggerOptions) *slog.Logger {
	writer := options.Writer
	if writer == nil {
		writer = os.Stdout
	}

	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: parseLevel(options.Level),
	})
	return slog.New(handler)
}

// parseLevel converte il livello testuale in un livello slog noto.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
