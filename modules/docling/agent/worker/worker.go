package worker

import (
	"context"
	aconfig "github.com/gaspardpetit/nfrx/modules/docling/agent/internal/config"
	internal "github.com/gaspardpetit/nfrx/modules/docling/agent/internal/worker"
)

type Config = aconfig.WorkerConfig

func Run(ctx context.Context, cfg Config) error { return internal.Run(ctx, cfg) }
