package llm

import (
	"strings"
	"testing"

	"github.com/gaspardpetit/nfrx/sdk/api/spi"
)

type testState struct{}

func (testState) IsDraining() bool { return false }
func (testState) SetStatus(string) {}

type testStateRegistry struct {
	elements map[string]spi.StateElement
}

func (r *testStateRegistry) Add(e spi.StateElement) {
	if r.elements == nil {
		r.elements = map[string]spi.StateElement{}
	}
	r.elements[e.ID] = e
}

func TestRegisterStateHTMLIncludesHostTelemetryFields(t *testing.T) {
	p := New(testState{}, "v", "sha", "date", spi.Options{}, nil)
	sr := &testStateRegistry{}
	p.RegisterState(sr)
	elem, ok := sr.elements["llm"]
	if !ok || elem.HTML == nil {
		t.Fatalf("missing llm state html")
	}
	html := elem.HTML()
	for _, want := range []string{"host_cpu_percent", "host_ram_used_percent", "host_hostname", "completion_agent_version", "input_tokens_total", "output_tokens_total", "Tokens In", "Tokens Out", "worker-version", "worker-hostline"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in html", want)
		}
	}
	if !strings.Contains(html, "hostParts=[hostName, w.host_os, w.host_platform, w.host_platform_version, completionAgentVersion].filter(Boolean)") {
		t.Fatalf("missing host line backend version composition")
	}
	if strings.Contains(html, "worker-backend") {
		t.Fatalf("unexpected separate backend line in html")
	}
}
