package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModels(t *testing.T) {
	t.Parallel()

	const payload = `{"object":"list","data":[{"id":"deepseek-r1:671b","object":"model","created":1763660652,"owned_by":"library"},{"id":"gpt-oss:20b","object":"model","created":1760491869,"owned_by":"library"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	client := New(srv.URL+"/v1", "secret")
	models, err := client.Models(context.Background())
	if err != nil {
		t.Fatalf("Models error: %v", err)
	}
	got := strings.Join(models, ",")
	if got != "deepseek-r1:671b,gpt-oss:20b" {
		t.Fatalf("unexpected models: %q", got)
	}
}
