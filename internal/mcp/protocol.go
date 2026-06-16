package mcp

import (
	"context"
	"encoding/json"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// handleMessage decodes one JSON-RPC message and returns a response when the
// protocol requires one.
func (s *Server) handleMessage(ctx context.Context, line []byte) (response, bool) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return errorResponse(nil, parseError, "invalid JSON-RPC message", nil), true
	}
	if len(req.ID) == 0 {
		// Notifications do not receive responses. MCP clients send
		// notifications/initialized after a successful initialize exchange.
		return response{}, false
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		return errorResponse(req.ID, invalidRequest, "invalid JSON-RPC request", nil), true
	}

	switch req.Method {
	case "initialize":
		return successResponse(req.ID, s.initializeResult()), true
	case "ping":
		return successResponse(req.ID, map[string]any{}), true
	case "tools/list":
		return successResponse(req.ID, map[string]any{"tools": tools()}), true
	case "tools/call":
		result, err := s.callTool(ctx, req.Params)
		if err != nil {
			return errorResponse(req.ID, invalidParams, err.Error(), nil), true
		}
		return successResponse(req.ID, result), true
	default:
		return errorResponse(req.ID, methodNotFound, "method not found", nil), true
	}
}

// initializeResult builds the MCP initialize payload advertised to clients.
func (s *Server) initializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    "kctx",
			"version": s.version,
		},
	}
}

// successResponse wraps a result in a JSON-RPC success response.
func successResponse(id json.RawMessage, result any) response {
	return response{JSONRPC: "2.0", ID: idOrNull(id), Result: result}
}

// errorResponse wraps an error in a JSON-RPC error response.
func errorResponse(id json.RawMessage, code int, message string, data any) response {
	return response{
		JSONRPC: "2.0",
		ID:      idOrNull(id),
		Error:   &responseError{Code: code, Message: message, Data: data},
	}
}

// idOrNull returns a JSON null id when no request id was available.
func idOrNull(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}
