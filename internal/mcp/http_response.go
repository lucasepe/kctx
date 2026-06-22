package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	data, err := s.marshalLimitedJSON(value)
	if err != nil {
		s.writeJSONLimitError(w, err)
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	return err
}

// marshalLimitedJSON encodes value as JSON after enforcing the configured cap.
func (s *HTTPServer) marshalLimitedJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode MCP response: %w", err)
	}
	if s.maxResponseBytes > 0 && int64(len(data)) > s.maxResponseBytes {
		return nil, fmt.Errorf("MCP response exceeds max response bytes: %d > %d", len(data), s.maxResponseBytes)
	}
	return data, nil
}

// writeJSONLimitError writes the JSON-RPC error response for a rejected response.
func (s *HTTPServer) writeJSONLimitError(w http.ResponseWriter, err error) {
	if strings.HasPrefix(err.Error(), "encode MCP response") {
		w.WriteHeader(http.StatusInternalServerError)
		_ = writeHTTPJSON(w, errorResponse(nil, invalidRequest, "encode MCP response", nil))
		return
	}
	resp := errorResponse(nil, invalidRequest, "MCP response exceeds max response bytes", map[string]string{
		"maxBytes": fmt.Sprintf("%d", s.maxResponseBytes),
	})
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_ = writeHTTPJSON(w, resp)
}

// writeHTTPJSON writes value as an application/json response.
func writeHTTPJSON(w http.ResponseWriter, value any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(value)
}
