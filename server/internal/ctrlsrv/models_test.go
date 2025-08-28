package ctrlsrv

import (
	"strings"
	"testing"
)

func TestAggregatedModels(t *testing.T) {
	reg := NewRegistry()
	reg.Add(&Worker{ID: "w1", Name: "Alpha", Models: map[string]bool{"llama3:8b": true, "mistral:7b": true}, MaxConcurrency: 1, EmbeddingBatchSize: 0})
	reg.Add(&Worker{ID: "w2", Name: "Beta", Models: map[string]bool{"llama3:8b": true, "qwen2.5:14b": true}, MaxConcurrency: 1, EmbeddingBatchSize: 0})

	list := reg.AggregatedModels()
	if len(list) != 3 {
		t.Fatalf("expected 3 models, got %d", len(list))
	}
	var found bool
	for _, m := range list {
		if m.ID == "llama3:8b" {
			found = true
			if owners := strings.Join(m.Owners, ","); owners != "Alpha,Beta" {
				t.Fatalf("owners wrong: %s", owners)
			}
			if m.Created <= 0 {
				t.Fatalf("created not set")
			}
		}
	}
	if !found {
		t.Fatalf("llama3:8b not found")
	}

	m, ok := reg.AggregatedModel("llama3:8b")
	if !ok {
		t.Fatalf("model not found")
	}
	if owners := strings.Join(m.Owners, ","); owners != "Alpha,Beta" {
		t.Fatalf("owners wrong: %s", owners)
	}
	if m.Created <= 0 {
		t.Fatalf("created not set")
	}
}
