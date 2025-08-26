package ctrlsrv_test

import (
	"context"
	"testing"

	ctrl "github.com/gaspardpetit/nfrx-sdk/ctrl"
	"github.com/gaspardpetit/nfrx-server/internal/controlgrpc"
	ctrlsrv "github.com/gaspardpetit/nfrx-server/internal/ctrlsrv"
)

func TestRegisterStoresWorkerName(t *testing.T) {
	reg := ctrlsrv.NewRegistry()
	metrics := ctrlsrv.NewMetricsRegistry("test", "", "")
	srv := controlgrpc.New(reg, metrics, "")
	req := &ctrl.RegisterRequest{WorkerId: "w1abcdef", Capabilities: []string{"llm"}, Metadata: map[string]string{"worker_name": "Alpha", "max_concurrency": "1", "embedding_batch_size": "0"}}
	if _, err := srv.Register(context.Background(), req); err != nil {
		t.Fatalf("register: %v", err)
	}
	if w, ok := reg.Get("w1abcdef"); !ok || w.Name != "Alpha" {
		t.Fatalf("expected name Alpha, got %v", w)
	}
}

func TestRegisterFallbackName(t *testing.T) {
	reg := ctrlsrv.NewRegistry()
	metrics := ctrlsrv.NewMetricsRegistry("test", "", "")
	srv := controlgrpc.New(reg, metrics, "")
	req := &ctrl.RegisterRequest{WorkerId: "w123456789", Capabilities: []string{"llm"}, Metadata: map[string]string{"max_concurrency": "1", "embedding_batch_size": "0"}}
	if _, err := srv.Register(context.Background(), req); err != nil {
		t.Fatalf("register: %v", err)
	}
	if w, ok := reg.Get("w123456789"); !ok || w.Name != "w1234567" {
		t.Fatalf("unexpected fallback name %q", w.Name)
	}
}
