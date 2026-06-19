package http

import (
	_ "embed"
	"net/http"
)

// openapiSpec — встроенная OpenAPI 3.0 спецификация API.
//
//go:embed docs/openapi.yaml
var openapiSpec []byte

// swaggerUIPage — страница Swagger UI. Ассеты подгружаются с CDN, спецификация
// берётся с эндпоинта /openapi.yaml того же сервиса.
const swaggerUIPage = `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <title>Team Manager API — Swagger UI</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script>
    window.onload = function () {
      window.ui = SwaggerUIBundle({
        url: "/openapi.yaml",
        dom_id: "#swagger-ui",
        presets: [SwaggerUIBundle.presets.apis],
        layout: "BaseLayout",
      });
    };
  </script>
</body>
</html>`

// handleOpenAPISpec отдаёт OpenAPI-спецификацию в формате YAML.
func (s *Server) handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapiSpec)
}

// handleSwaggerUI отдаёт страницу Swagger UI.
func (s *Server) handleSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerUIPage))
}
