// Package api embeds the OpenAPI contract and the Redoc HTML renderer
// so the binary serves its own documentation without runtime file access.
// Other packages depend on OpenAPIYAML and RedocHTML for wiring.
package api

import _ "embed"

// OpenAPIYAML is the raw bytes of the embedded OpenAPI 3.1 specification,
// served as-is on GET /openapi.yaml.
//
//go:embed openapi.yaml
var OpenAPIYAML []byte

// RedocHTML is the static HTML page that renders OpenAPIYAML via Redoc.
// Served as-is on GET /docs.
//
//go:embed redoc.html
var RedocHTML []byte
