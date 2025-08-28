package serverstate

import (
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
)

func TestRedisStore(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	rs, err := NewRedisStore(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}

	prev := active
	UseStore(rs)
	defer UseStore(prev)

	if got := GetState(); got != "not_ready" {
		t.Fatalf("initial state = %q; want %q", got, "not_ready")
	}

	SetState("ready")
	if got := GetState(); got != "ready" {
		t.Fatalf("state after SetState = %q; want %q", got, "ready")
	}

	StartDrain()
	if got := GetState(); got != "draining" {
		t.Fatalf("state after StartDrain = %q; want %q", got, "draining")
	}
	if !IsDraining() {
		t.Fatalf("IsDraining = false; want true")
	}

	// Ensure a new store sees the persisted state.
	rs2, err := NewRedisStore(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	if st := rs2.Load(); st.Status != "draining" || !st.Draining {
		t.Fatalf("persisted state = %#v; want {Status: 'draining', Draining: true}", st)
	}
}

func TestParseRedisURL(t *testing.T) {
	tests := []struct {
		url    string
		addrs  int
		master string
		db     int
	}{
		{"redis://:pass@localhost:6379/1", 1, "", 1},
		{"redis://host1:6379,host2:6379/0", 2, "", 0},
		{"redis-sentinel://localhost:26379/mymaster?db=2", 1, "mymaster", 2},
	}
	for _, tt := range tests {
		opts, err := parseRedisURL(tt.url)
		if err != nil {
			t.Fatalf("parseRedisURL(%q): %v", tt.url, err)
		}
		if len(opts.Addrs) != tt.addrs {
			t.Fatalf("%q addrs = %d; want %d", tt.url, len(opts.Addrs), tt.addrs)
		}
		if opts.MasterName != tt.master {
			t.Fatalf("%q master = %q; want %q", tt.url, opts.MasterName, tt.master)
		}
		if opts.DB != tt.db {
			t.Fatalf("%q db = %d; want %d", tt.url, opts.DB, tt.db)
		}
	}
}
