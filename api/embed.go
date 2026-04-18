// Package api embeds the OpenAPI contract and the Redoc HTML renderer
// so the binary serves its own documentation without runtime file access.
// Other packages depend on OpenAPIYAML and RedocHTML for wiring.
package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPIYAML []byte

//go:embed redoc.html
var RedocHTML []byte
