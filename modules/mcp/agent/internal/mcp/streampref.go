package mcp

import "sync"

type streamPref struct {
	mu    sync.RWMutex
	allow bool
}

func newStreamPref(defaultAllow bool) *streamPref {
	return &streamPref{allow: defaultAllow}
}

func (s *streamPref) Allow() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.allow
}

func (s *streamPref) Set(v bool) {
	s.mu.Lock()
	s.allow = v
	s.mu.Unlock()
}
