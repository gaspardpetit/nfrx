package worker

import (
	"context"

	aconfig "github.com/gaspardpetit/nfrx/modules/llm/agent/internal/config"
	internal "github.com/gaspardpetit/nfrx/modules/llm/agent/internal/worker"
)

// Config re-exports the worker configuration.
type Config = aconfig.WorkerConfig

// Run starts the LLM worker agent.
func Run(ctx context.Context, cfg Config) error {
	return internal.Run(ctx, cfg)
}
