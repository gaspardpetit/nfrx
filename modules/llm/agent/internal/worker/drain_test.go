package worker

import "testing"

func TestStartDrain(t *testing.T) {
	resetState()
	SetConnectedToServer(true)
	SetState("connected_idle")
	StartDrain()
	if !IsDraining() || GetState().State != "draining" {
		t.Fatalf("unexpected state: %+v", GetState())
	}
	IncJobs()
	_ = DecJobs()
	if GetState().State != "draining" {
		t.Fatalf("state should remain draining")
	}
}
