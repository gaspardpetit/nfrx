package api

import (
	_ "embed"
	"net/http"

	"github.com/you/llamapool/internal/logx"
)

//go:embed openapi.json
var openapiJSON []byte

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
