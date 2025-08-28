package mcpbridge

import (
	"encoding/json"
	"testing"
)

func TestFramePayloadRoundTrip(t *testing.T) {
	raw := json.RawMessage(`{"jsonrpc":"2.0","method":"ping","params":{"a":1}}`)
	f := Frame{Type: TypeRequest, ID: "1", SessionID: "s", Payload: raw}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Frame
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(got.Payload) != string(raw) {
		t.Fatalf("payload changed: %s != %s", got.Payload, raw)
	}
}
