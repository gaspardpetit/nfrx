package logx

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Log is the shared logger used throughout the project.
var Log = log.Logger

// Configure sets the global log level and output format.
// The level string is tolerant of case and common synonyms.
func Configure(level string) {
	zerolog.SetGlobalLevel(parseLevel(level))

	// Optional: make logs human-readable in dev
	Log = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

// parseLevel converts a string to a zerolog level.
// Accepts: all, debug, info, warn, warning, error, fatal, none.
// Unknown values default to info.
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "all", "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "none", "off", "disabled":
		return zerolog.Disabled
	default:
		return zerolog.InfoLevel
	}
}

func init() {
	Configure(os.Getenv("LOG_LEVEL"))
}
