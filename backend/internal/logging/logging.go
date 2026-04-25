// Package logging wires the process-wide slog logger.
//
// The AccessLog middleware already emits one JSON object per request when
// PMS_ACCESS_LOG_FORMAT=json; this package covers the remainder of the code
// that still calls log.Printf. Init() replaces the default log.Default()
// writer with one that forwards to slog, so existing log.Printf call sites
// gain structured output without touching every file.
//
// The top-level slog.Default() handler is JSON whenever PMS_LOG_FORMAT=json
// or PMS_ENV!=dev/test; otherwise it stays as a human-readable text handler
// to keep local development ergonomic.
package logging

import (
	"log"
	"log/slog"
	"os"
	"strings"
)

// Init configures slog as the process default and redirects the standard
// `log` package so callers using log.Printf automatically get structured
// output. Call once at startup, before any goroutines fan out.
func Init(env string) *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	opts := &slog.HandlerOptions{Level: level}

	format := strings.ToLower(strings.TrimSpace(os.Getenv("PMS_LOG_FORMAT")))
	useJSON := format == "json" || (format == "" && env != "dev" && env != "development" && env != "test")

	var handler slog.Handler
	if useJSON {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Bridge the legacy log package. log.Printf calls are now emitted via
	// slog at INFO level, preserving the existing message format.
	log.SetFlags(0)
	log.SetOutput(&slogWriter{logger: logger})
	return logger
}

// slogWriter adapts io.Writer semantics to slog so legacy log.Printf calls
// end up in the structured pipeline without rewriting them all at once.
type slogWriter struct{ logger *slog.Logger }

func (s *slogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	s.logger.Info(msg, slog.String("source", "log_printf"))
	return len(p), nil
}

func parseLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
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
