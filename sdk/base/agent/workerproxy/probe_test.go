package workerproxy

import (
    "context"
    "errors"
    "testing"
)

func TestProbeFuncUpdatesStateAndModels(t *testing.T) {
    cfg := Config{ClientID: "w1", ClientName: "n", MaxConcurrency: 2}
    // Ready with one model
    cfg.ProbeFunc = func(ctx context.Context) (ProbeResult, error) { return ProbeResult{Ready: true, Models: []string{"m1"}, MaxConcurrency: 2}, nil }
    if err := probeBackend(context.Background(), cfg, nil); err != nil {
        t.Fatalf("probe ready: %v", err)
    }
    s := GetState()
    if !s.ConnectedToBackend || len(s.Labels) != 1 || s.Labels[0] != "m1" { t.Fatalf("bad state: %+v", s) }

    // Change models
    cfg.ProbeFunc = func(ctx context.Context) (ProbeResult, error) { return ProbeResult{Ready: true, Models: []string{"m1", "m2"}, MaxConcurrency: 2}, nil }
    if err := probeBackend(context.Background(), cfg, nil); err != nil {
        t.Fatalf("probe update: %v", err)
    }
    s = GetState()
    if len(s.Labels) != 2 || s.Labels[1] != "m2" { t.Fatalf("labels not updated: %+v", s.Labels) }

    // Failure
    cfg.ProbeFunc = func(ctx context.Context) (ProbeResult, error) { return ProbeResult{Ready: false}, errors.New("down") }
    if err := probeBackend(context.Background(), cfg, nil); err == nil { t.Fatalf("expected error") }
    s = GetState()
    if s.ConnectedToBackend || s.LastError == "" { t.Fatalf("expected failure: %+v", s) }
}
