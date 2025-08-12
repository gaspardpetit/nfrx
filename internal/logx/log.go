package logx

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Log is the shared logger used throughout the project.
var Log = log.Logger

func init() {
	if strings.ToLower(os.Getenv("DEBUG")) == "true" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Optional: make logs human-readable in dev
	Log = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}
