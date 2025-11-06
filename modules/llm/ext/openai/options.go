package openai

import "time"

// Options controls OpenAI-surface specifics.
type Options struct {
    RequestTimeout        time.Duration
    MaxParallelEmbeddings int
    // QueueSize caps the number of queued chat completion requests (0 disables queuing).
    QueueSize             int
    // QueueUpdateSeconds controls how often to emit queued status SSE lines (0 disables updates).
    QueueUpdateSeconds    int
}
