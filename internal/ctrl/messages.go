package ctrl

import "encoding/json"

type RegisterMessage struct {
	Type           string   `json:"type"`
	WorkerID       string   `json:"worker_id"`
	Token          string   `json:"token"`
	Models         []string `json:"models"`
	MaxConcurrency int      `json:"max_concurrency"`
}

type HeartbeatMessage struct {
	Type string `json:"type"`
	TS   int64  `json:"ts"`
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
