package test

import (
	"github.com/gaspardpetit/nfrx/modules/common/spi"
	openai "github.com/gaspardpetit/nfrx/modules/llm/ext/openai"
	"github.com/gaspardpetit/nfrx/server/internal/config"
	llm "github.com/gaspardpetit/nfrx/server/internal/llm"
	"github.com/go-chi/chi/v5"
)

func openAIMount(cfg config.ServerConfig) llm.APIMount {
	return func(v1 chi.Router, reg spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics) {
		openai.Mount(v1, reg, sched, metrics, openai.Options{RequestTimeout: cfg.RequestTimeout, MaxParallelEmbeddings: cfg.MaxParallelEmbeddings})
	}
}
