package mcp

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
