package handler

import (
	_ "embed"
	"net/http"
)

//go:embed docs.html
var docsHTML []byte

//go:embed docs/swagger.yaml
var swaggerYAML []byte

// DocsHandler serves the Swagger UI documentation page and the raw OpenAPI 3.0 specification.
type DocsHandler struct{}

// NewDocsHandler creates a new instance of DocsHandler.
func NewDocsHandler() *DocsHandler {
	return &DocsHandler{}
}

// ServeDocsHTML handles GET /docs.
// Serves the custom-styled Swagger UI HTML page.
func (h *DocsHandler) ServeDocsHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(docsHTML)
}

// ServeSwaggerYAML handles GET /api/v1/swagger.yaml.
// Serves the raw swagger.yaml spec file.
func (h *DocsHandler) ServeSwaggerYAML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(swaggerYAML)
}
