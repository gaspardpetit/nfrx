package api

import (
	"encoding/json"
	"net/http"

	"github.com/you/llamapool/internal/logx"
)

var openapiJSON = mustOpenAPISchema()

func mustOpenAPISchema() []byte {
	schema := map[string]any{
		"openapi": "3.0.0",
		"info": map[string]any{
			"title":   "llamapool API",
			"version": "1.0.0",
		},
		"paths": map[string]any{
			"/api/generate": map[string]any{
				"post": map[string]any{
					"summary": "Generate content",
					"responses": map[string]any{
						"200": map[string]any{"description": "Generated response"},
					},
				},
			},
			"/api/tags": map[string]any{
				"get": map[string]any{
					"summary": "List models",
					"responses": map[string]any{
						"200": map[string]any{"description": "List of models"},
					},
				},
			},
			"/api/v1/state": map[string]any{
				"get": map[string]any{
					"summary": "Get server state",
					"responses": map[string]any{
						"200": map[string]any{"description": "State"},
					},
				},
			},
			"/api/v1/state/stream": map[string]any{
				"get": map[string]any{
					"summary": "Stream server state",
					"responses": map[string]any{
						"200": map[string]any{"description": "Event stream"},
					},
				},
			},
			"/v1/models": map[string]any{
				"get": map[string]any{
					"summary": "List models",
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
					},
				},
			},
			"/v1/models/{id}": map[string]any{
				"get": map[string]any{
					"summary": "Get model",
					"parameters": []any{
						map[string]any{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "string"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
					},
				},
			},
			"/v1/chat/completions": map[string]any{
				"post": map[string]any{
					"summary": "Chat completions",
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
					},
				},
			},
			"/healthz": map[string]any{
				"get": map[string]any{
					"summary": "Health check",
					"responses": map[string]any{
						"200": map[string]any{"description": "OK"},
					},
				},
			},
		},
	}
	b, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	return b
}

// OpenAPIHandler serves the embedded OpenAPI schema.
func OpenAPIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(openapiJSON); err != nil {
			// ignore error but log
			logx.Log.Error().Err(err).Msg("write openapi")
		}
	}
}

const swaggerPage = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>llamapool API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
  window.onload = () => {
    SwaggerUIBundle({
      url: 'openapi.json',
      dom_id: '#swagger-ui'
    });
  };
  </script>
</body>
</html>`

// SwaggerHandler serves a minimal Swagger UI.
func SwaggerHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write([]byte(swaggerPage)); err != nil {
			// ignore error but log
			logx.Log.Error().Err(err).Msg("write swagger page")
		}
	}
}
