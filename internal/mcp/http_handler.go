package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

// handleStreamableHTTP routes Streamable HTTP requests after applying the
// cross-origin and protocol-version checks required by browser clients.
func (s *HTTPServer) handleStreamableHTTP(w http.ResponseWriter, r *http.Request) {
	if !validOrigin(r) {
		http.Error(w, "invalid Origin", http.StatusForbidden)
		return
	}
	setStreamableHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !validProtocolVersion(r) {
		http.Error(w, "invalid MCP protocol version", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handleStreamablePost(w, r)
	case http.MethodGet:
		// GET is optional in Streamable HTTP. kctx returns direct JSON responses to
		// POST requests and does not currently support server-to-client streams,
		// resumability, or Last-Event-ID replay.
		http.Error(w, "MCP Streamable HTTP GET streams are not supported", http.StatusMethodNotAllowed)
	case http.MethodDelete:
		s.handleStreamableDelete(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleStreamablePost processes one JSON-RPC message sent over Streamable HTTP.
func (s *HTTPServer) handleStreamablePost(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	if s.maxRequestBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxRequestBytes)
	}

	var req json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_ = writeHTTPJSON(w, errorResponse(nil, invalidRequest, "MCP request exceeds max request bytes", map[string]string{
				"maxBytes": fmt.Sprintf("%d", maxBytesErr.Limit),
			}))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = writeHTTPJSON(w, errorResponse(nil, parseError, "invalid JSON-RPC message", nil))
		return
	}

	meta := streamableMessageMetadata(req)
	if !meta.isInitialize() && !s.validStreamableSession(r) {
		http.Error(w, "missing or expired MCP session", http.StatusBadRequest)
		return
	}
	if meta.isResponse() {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp, respond := s.server.handleMessage(r.Context(), req)
	if !respond {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if meta.isInitialize() && resp.Error == nil {
		session, err := s.streamableSessions.Create(r.Context())
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = writeHTTPJSON(w, errorResponse(nil, invalidRequest, "cannot create MCP session", nil))
			return
		}
		w.Header().Set(mcpSessionIDHeader, session.ID)
	}
	if err := s.writeLimitedJSON(w, resp); err != nil && s.logger != nil {
		s.logger.Warn("mcp http response rejected", slog.String("err", err.Error()))
	}
}

// handleStreamableDelete terminates one Streamable HTTP session.
func (s *HTTPServer) handleStreamableDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(mcpSessionIDHeader)
	if sessionID == "" {
		http.Error(w, "missing MCP session", http.StatusBadRequest)
		return
	}
	if !s.streamableSessions.Delete(sessionID) {
		http.Error(w, "missing or expired MCP session", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validStreamableSession reports whether the request carries a live MCP session.
func (s *HTTPServer) validStreamableSession(r *http.Request) bool {
	sessionID := r.Header.Get(mcpSessionIDHeader)
	if sessionID == "" {
		return false
	}
	_, ok := s.streamableSessions.Get(sessionID)
	return ok
}

type streamableMessageMeta struct {
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

// streamableMessageMetadata extracts only the fields needed to route the message.
func streamableMessageMetadata(raw json.RawMessage) streamableMessageMeta {
	var meta streamableMessageMeta
	_ = json.Unmarshal(raw, &meta)
	return meta
}

// isInitialize reports whether the message is an MCP initialize request.
func (m streamableMessageMeta) isInitialize() bool {
	return m.Method == "initialize"
}

// isResponse reports whether the message is a JSON-RPC response sent by a client.
func (m streamableMessageMeta) isResponse() bool {
	return m.Method == "" && (len(m.Result) > 0 || len(m.Error) > 0)
}
