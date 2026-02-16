package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTags(t *testing.T) {
	t.Parallel()

	const payload = `{"models":[{"name":"deepseek-r1:671b","model":"deepseek-r1:671b","modified_at":"2025-11-20T12:44:12.349081345-05:00","size":404430190268},{"name":"gpt-oss:20b","model":"gpt-oss:20b","modified_at":"2025-10-14T21:31:09.604234083-04:00","size":13793441244}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/tags" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	client := New(srv.URL)
	models, err := client.Tags(context.Background())
	if err != nil {
		t.Fatalf("Tags error: %v", err)
	}
	got := strings.Join(models, ",")
	if got != "deepseek-r1:671b,gpt-oss:20b" {
		t.Fatalf("unexpected models: %q", got)
	}
}
