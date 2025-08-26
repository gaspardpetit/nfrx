package llmagent

import (
	"testing"
	"time"
)

func TestStateTransitions(t *testing.T) {
	resetState()
	SetConnectedToServer(true)
	SetState("connected_idle")
	IncJobs()
	s := GetState()
	if s.CurrentJobs != 1 || s.State != "connected_busy" {
		t.Fatalf("expected busy state, got %+v", s)
	}
	_ = DecJobs()
	s = GetState()
	if s.CurrentJobs != 0 || s.State != "connected_idle" {
		t.Fatalf("expected idle state, got %+v", s)
	}
	now := time.Now()
	SetLastHeartbeat(now)
	if !GetState().LastHeartbeat.Equal(now) {
		t.Fatalf("heartbeat not set")
	}
}
