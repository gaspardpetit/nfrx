package ctrl

import "encoding/json"

type RegisterMessage struct {
	Type           string   `json:"type"`
	WorkerID       string   `json:"worker_id"`
	WorkerName     string   `json:"worker_name,omitempty"`
	ClientKey      string   `json:"client_key"`
	Token          string   `json:"token,omitempty"`
	Models         []string `json:"models"`
	MaxConcurrency int      `json:"max_concurrency"`
	Version        string   `json:"version,omitempty"`
	BuildSHA       string   `json:"build_sha,omitempty"`
	BuildDate      string   `json:"build_date,omitempty"`
}

type StatusUpdateMessage struct {
	Type           string   `json:"type"`
	MaxConcurrency int      `json:"max_concurrency"`
	Models         []string `json:"models,omitempty"`
	Status         string   `json:"status,omitempty"`
}

type HeartbeatMessage struct {
	Type string `json:"type"`
	TS   int64  `json:"ts"`
}

type ModelsUpdateMessage struct {
	Type   string   `json:"type"`
	Models []string `json:"models"`
}

type JobChunkMessage struct {
	Type  string          `json:"type"`
	JobID string          `json:"job_id"`
	Data  json.RawMessage `json:"data"`
}

type JobResultMessage struct {
	Type  string          `json:"type"`
	JobID string          `json:"job_id"`
	Data  json.RawMessage `json:"data"`
}

type JobErrorMessage struct {
	Type    string `json:"type"`
	JobID   string `json:"job_id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type JobRequestMessage struct {
	Type     string      `json:"type"`
	JobID    string      `json:"job_id"`
	Endpoint string      `json:"endpoint"`
	Payload  interface{} `json:"payload"`
}

type CancelJobMessage struct {
	Type  string `json:"type"`
	JobID string `json:"job_id"`
}

type HTTPProxyRequestMessage struct {
	Type      string            `json:"type"`
	RequestID string            `json:"request_id"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Headers   map[string]string `json:"headers,omitempty"`
	Stream    bool              `json:"stream,omitempty"`
	Body      []byte            `json:"body,omitempty"`
}

type HTTPProxyResponseHeadersMessage struct {
	Type      string            `json:"type"`
	RequestID string            `json:"request_id"`
	Status    int               `json:"status"`
	Headers   map[string]string `json:"headers,omitempty"`
}

type HTTPProxyResponseChunkMessage struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Data      []byte `json:"data"`
}

type HTTPProxyError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type HTTPProxyResponseEndMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Error     *HTTPProxyError `json:"error,omitempty"`
}

type HTTPProxyCancelMessage struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}
