// Package httpx contains HTTP-layer utilities shared across the service:
// RFC 7807 problem responses, common middlewares, and context helpers.
// It is deliberately independent of any application domain.
package httpx

import (
	"encoding/json"
	"net/http"
)

// ProblemContentType is the media type defined by RFC 7807.
const ProblemContentType = "application/problem+json"

// Problem is an RFC 7807 "Problem Details" value. Client-facing fields
// follow the RFC; RequestID is an extension for log correlation.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Instance  string `json:"instance,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteProblem serialises p to w with the RFC 7807 content type and the
// status carried by p. A non-nil encoder error is intentionally ignored:
// the header was already flushed when we get here, so there is nothing
// useful we can do other than leave the body truncated.
func WriteProblem(w http.ResponseWriter, p Problem) {
	if p.Type == "" {
		p.Type = "about:blank"
	}
	w.Header().Set("Content-Type", ProblemContentType)
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
