package openai

import "time"

type Options struct {
	RequestTimeout        time.Duration
	MaxParallelEmbeddings int
}
