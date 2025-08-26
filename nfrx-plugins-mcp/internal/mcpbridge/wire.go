package mcpbridge

import mcpwire "github.com/gaspardpetit/nfrx-sdk/mcpwire"

type Frame = mcpwire.BridgeFrame

type FrameType = mcpwire.FrameType

const (
	TypeRequest        = mcpwire.TypeRequest
	TypeResponse       = mcpwire.TypeResponse
	TypeNotification   = mcpwire.TypeNotification
	TypeServerRequest  = mcpwire.TypeServerRequest
	TypeServerResponse = mcpwire.TypeServerResponse
	TypeStreamEvent    = mcpwire.TypeStreamEvent
)
