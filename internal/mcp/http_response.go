package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// livez writes a minimal liveness response for Kubernetes probes.
func (s *HTTPServer) livez(w http.ResponseWriter, r *http.Request) {
	writeHTTPJSON(w, map[string]string{"status": "alive"})
}

// readyz writes readiness based on whether the MCP server has an engine.
func (s *HTTPServer) readyz(w http.ResponseWriter, r *http.Request) {
	if s.server == nil || s.server.engine == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = writeHTTPJSON(w, map[string]string{"status": "not_ready"})
		return
	}
	writeHTTPJSON(w, map[string]string{"status": "ready"})
}

// version writes the kctx server version advertised by this process.
func (s *HTTPServer) version(w http.ResponseWriter, r *http.Request) {
	version := defaultVersion
	if s.server != nil && s.server.version != "" {
		version = s.server.version
	}
	writeHTTPJSON(w, map[string]string{"name": "kctx", "version": version})
}

// writeLimitedJSON writes a JSON response after enforcing the configured cap.
func (s *HTTPServer) writeLimitedJSON(w http.ResponseWriter, value any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(value); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = writeHTTPJSON(w, errorResponse(nil, invalidRequest, "encode MCP response", nil))
		return err
	}
	if s.maxResponseBytes > 0 && int64(buf.Len()) > s.maxResponseBytes {
		resp := errorResponse(nil, invalidRequest, "MCP response exceeds max response bytes", map[string]string{
			"maxBytes": fmt.Sprintf("%d", s.maxResponseBytes),
			"bytes":    fmt.Sprintf("%d", buf.Len()),
		})
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_ = writeHTTPJSON(w, resp)
		return fmt.Errorf("MCP response exceeds max response bytes: %d > %d", buf.Len(), s.maxResponseBytes)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write(buf.Bytes())
	return err
}

// writeHTTPJSON writes value as an application/json response.
func writeHTTPJSON(w http.ResponseWriter, value any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(value)
}
