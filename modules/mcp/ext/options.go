package mcp

import "time"

type Options struct {
	RequestTimeout time.Duration
	ClientKey      string
}
