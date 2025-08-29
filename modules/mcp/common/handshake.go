package common

// Register is the initial register frame from MCP agent to server.
type Register struct {
	ID         string `json:"id"`
	ClientName string `json:"client_name"`
	ClientKey  string `json:"client_key"`
}

// Ack is the server's acknowledgement with assigned ID.
type Ack struct {
	ID string `json:"id"`
}
