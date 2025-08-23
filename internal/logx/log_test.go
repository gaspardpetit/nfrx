package logx_test

import (
	"testing"

	"github.com/gaspardpetit/infero/internal/logx"
	"github.com/rs/zerolog"
)

func TestConfigureLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "trace")
	t.Setenv("DEBUG", "")
	logx.Configure()
	if zerolog.GlobalLevel() != zerolog.TraceLevel {
		t.Fatalf("expected trace level, got %s", zerolog.GlobalLevel())
	}

	t.Setenv("LOG_LEVEL", "")
	t.Setenv("DEBUG", "true")
	logx.Configure()
	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Fatalf("expected debug level, got %s", zerolog.GlobalLevel())
	}

	t.Setenv("LOG_LEVEL", "bogus")
	t.Setenv("DEBUG", "")
	logx.Configure()
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Fatalf("expected info level, got %s", zerolog.GlobalLevel())
	}
}
