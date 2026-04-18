package httpx

import "net/http"

// OpenAPIHandler serves the embedded OpenAPI document with the
// application/yaml content type, suitable for consumption by Redoc,
// Swagger UI or any OpenAPI-aware tool.
func OpenAPIHandler(spec []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(spec)
	}
}

// RedocHandler serves a static HTML page that renders the OpenAPI
// document via Redoc. The Redoc bundle itself is loaded from the CDN
// referenced in the HTML.
func RedocHandler(html []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(html)
	}
}
