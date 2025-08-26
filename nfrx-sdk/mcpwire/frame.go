package mcpwire

import "encoding/json"

// Frame represents a broker <-> relay frame.
type Frame struct {
	T       string          `json:"t"`
	SID     string          `json:"sid,omitempty"`
	ReqID   string          `json:"req_id,omitempty"`
	Hint    string          `json:"hint,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Code    string          `json:"code,omitempty"`
	Msg     string          `json:"msg,omitempty"`
	Auth    string          `json:"auth,omitempty"`
}

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

// BridgeFrame carries an opaque JSON-RPC payload across the bridge.
type BridgeFrame struct {
	Type      FrameType       `json:"type"`
	ID        string          `json:"id,omitempty"`
	SessionID string          `json:"sessionId"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}
