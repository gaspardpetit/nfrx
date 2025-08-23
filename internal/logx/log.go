package logx

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Log is the shared logger used throughout the project.
var Log = log.Logger

// Configure sets the global log level based on environment variables.
// LOG_LEVEL takes precedence and accepts zerolog levels (trace, debug, info,
// warn, error, fatal, panic). When unset, DEBUG=true maps to debug level,
// otherwise info is used. Invalid values fall back to info.
func Configure() {
	lvlStr := strings.ToLower(os.Getenv("LOG_LEVEL"))
	if lvlStr == "" {
		if strings.ToLower(os.Getenv("DEBUG")) == "true" {
			lvlStr = "debug"
		} else {
			lvlStr = "info"
		}
	}
	lvl, err := zerolog.ParseLevel(lvlStr)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	// Optional: make logs human-readable in dev
	Log = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

func init() {
	Configure()
}
