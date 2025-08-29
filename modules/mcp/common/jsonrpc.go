package common

import "encoding/json"

// ValidateEnvelope checks a basic JSON-RPC 2.0 envelope.
func ValidateEnvelope(body []byte) (id any, method string, ok bool) {
	var env struct {
		JSONRPC string `json:"jsonrpc"`
		ID      any    `json:"id"`
		Method  string `json:"method"`
	}
	if json.Unmarshal(body, &env) != nil {
		return nil, "", false
	}
	if env.JSONRPC != "2.0" || env.ID == nil || env.Method == "" {
		return nil, "", false
	}
	return env.ID, env.Method, true
}
