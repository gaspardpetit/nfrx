package mcpbridge

import "encoding/json"

// FrameType enumerates bridge frame types.
type FrameType string

const (
	TypeRequest        FrameType = "request"
	TypeResponse       FrameType = "response"
	TypeNotification   FrameType = "notification"
	TypeServerRequest  FrameType = "server_request"
	TypeServerResponse FrameType = "server_response"
	TypeStreamEvent    FrameType = "stream_event"
)

// Frame carries an opaque JSON-RPC payload across the bridge.
type Frame struct {
	Type      FrameType       `json:"type"`
	ID        string          `json:"id,omitempty"`
	SessionID string          `json:"sessionId"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}
