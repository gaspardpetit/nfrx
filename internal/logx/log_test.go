package logx_test

import (
	"testing"

	"github.com/gaspardpetit/nfrx/internal/logx"
	"github.com/rs/zerolog"
)

func TestConfigureLogLevel(t *testing.T) {
	logx.Configure("all")
	if zerolog.GlobalLevel() != zerolog.TraceLevel {
		t.Fatalf("expected trace level, got %s", zerolog.GlobalLevel())
	}

	logx.Configure("WARNING")
	if zerolog.GlobalLevel() != zerolog.WarnLevel {
		t.Fatalf("expected warn level, got %s", zerolog.GlobalLevel())
	}

	logx.Configure("none")
	if zerolog.GlobalLevel() != zerolog.Disabled {
		t.Fatalf("expected disabled level, got %s", zerolog.GlobalLevel())
	}

	logx.Configure("bogus")
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Fatalf("expected info level, got %s", zerolog.GlobalLevel())
	}
}
