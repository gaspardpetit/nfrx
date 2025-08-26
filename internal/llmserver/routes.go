package llmserver

import (
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/gaspardpetit/nfrx/api/generated"
	"github.com/gaspardpetit/nfrx/internal/api"
	ctrlsrv "github.com/gaspardpetit/nfrx/internal/ctrlsrv"
)

// RegisterRoutes wires the HTTP endpoints.
func (p *Plugin) RegisterRoutes(r chi.Router) {
	impl := &api.API{Reg: p.reg, Metrics: p.metrics, MCP: p.mcp, Sched: p.sched, Timeout: p.cfg.RequestTimeout, MaxParallelEmbeddings: p.cfg.MaxParallelEmbeddings}
	wrapper := generated.ServerInterfaceWrapper{Handler: impl}

	r.Get("/healthz", wrapper.GetHealthz)
	r.Route("/api", func(apiGroup chi.Router) {
		if p.cfg.APIKey != "" {
			apiGroup.Use(api.APIKeyMiddleware(p.cfg.APIKey))
		}
		apiGroup.Route("/v1", func(v1 chi.Router) {
			v1.Post("/chat/completions", wrapper.PostApiV1ChatCompletions)
			v1.Post("/embeddings", wrapper.PostApiV1Embeddings)
			v1.Get("/models", wrapper.GetApiV1Models)
			v1.Get("/models/{id}", wrapper.GetApiV1ModelsId)
		})
		apiGroup.Get("/state", wrapper.GetApiState)
		apiGroup.Get("/state/stream", wrapper.GetApiStateStream)
	})
	r.Route("/api/client", func(r chi.Router) {
		r.Get("/openapi.json", api.OpenAPIHandler())
		r.Get("/*", api.SwaggerHandler())
	})

	go func() {
		ticker := time.NewTicker(ctrlsrv.HeartbeatInterval)
		for range ticker.C {
			p.reg.PruneExpired(ctrlsrv.HeartbeatExpiry)
		}
	}()
}

// RegisterWebSocket attaches the worker connect endpoint.
func (p *Plugin) RegisterWebSocket(r chi.Router) {
	r.Handle("/api/workers/connect", ctrlsrv.WSHandler(p.reg, p.metrics, p.cfg.ClientKey))
}
