package mcpbridge

import (
	"encoding/json"
	"testing"
)

func TestIDMapper(t *testing.T) {
	m := NewIDMapper()
	orig := json.RawMessage(`"abc"`)
	corr := m.Alloc(orig)
	if corr == "" {
		t.Fatal("empty corr id")
	}
	got, ok := m.Resolve(corr)
	if !ok {
		t.Fatal("id not found")
	}
	if string(got) != string(orig) {
		t.Fatalf("expected %s got %s", orig, got)
	}
	if _, ok := m.Resolve(corr); ok {
		t.Fatal("mapping should be removed after resolve")
	}
}

func TestIDMapperNumericID(t *testing.T) {
	m := NewIDMapper()
	orig := json.RawMessage(`123`)
	corr := m.Alloc(orig)
	got, ok := m.Resolve(corr)
	if !ok || string(got) != "123" {
		t.Fatalf("expected 123 got %s ok=%v", got, ok)
	}
}
