// Package mcp exposes the kctx engine through a small Model Context Protocol
// server suitable for local AI-agent integrations.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/limits"
)

const (
	protocolVersion = "2025-06-18"
	defaultVersion  = "dev"
	maxEventLimit   = 500

	parseError     = -32700
	invalidRequest = -32600
	methodNotFound = -32601
	invalidParams  = -32602
)

// Server serves kctx tools over newline-delimited JSON-RPC 2.0 messages on
// stdio. The implementation intentionally supports the small MCP surface kctx
// needs first: initialization, tool listing, and tool calls.
type Server struct {
	engine         *engine.Engine
	logger         *slog.Logger
	version        string
	requestTimeout time.Duration
	kubeAPIBudget  int
}

// Option configures the MCP server.
type Option func(*Server)

// WithRequestTimeout sets the per-tool-call deadline. A zero value disables the
// deadline, matching the HTTP server option.
func WithRequestTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.requestTimeout = timeout
	}
}

// WithKubeAPIBudget limits Kubernetes API calls per tool invocation. A zero
// value disables the guard.
func WithKubeAPIBudget(limit int) Option {
	return func(s *Server) {
		s.kubeAPIBudget = limit
	}
}

// WithVersion sets the version advertised through MCP initialize.
func WithVersion(version string) Option {
	return func(s *Server) {
		if version != "" {
			s.version = version
		}
	}
}

// New creates an MCP server backed by eng.
func New(eng *engine.Engine, logger *slog.Logger, opts ...Option) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		engine:         eng,
		logger:         logger,
		version:        defaultVersion,
		requestTimeout: 30 * time.Second,
		kubeAPIBudget:  100,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

// Serve reads MCP JSON-RPC messages from in and writes responses to out. It
// returns nil on clean EOF so CLI shutdown behaves like other stdio tools.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		resp, ok := s.handleMessage(ctx, line)
		if !ok {
			continue
		}
		if err := encoder.Encode(resp); err != nil {
			return fmt.Errorf("write MCP response: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read MCP request: %w", err)
	}
	return nil
}

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

func successResponse(id json.RawMessage, result any) response {
	return response{JSONRPC: "2.0", ID: idOrNull(id), Result: result}
}

func errorResponse(id json.RawMessage, code int, message string, data any) response {
	return response{
		JSONRPC: "2.0",
		ID:      idOrNull(id),
		Error:   &responseError{Code: code, Message: message, Data: data},
	}
}

func idOrNull(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}

type callToolParams struct {
	Name     string          `json:"name"`
	Argument json.RawMessage `json:"arguments"`
}

func (s *Server) callTool(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	if s.engine == nil {
		return toolResult{}, fmt.Errorf("engine is not configured")
	}
	var params callToolParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return toolResult{}, fmt.Errorf("invalid tools/call params")
	}
	if params.Name == "" {
		return toolResult{}, fmt.Errorf("tool name is required")
	}
	ctx, cancel := s.toolContext(ctx)
	defer cancel()

	start := time.Now()
	result, err := s.dispatchTool(ctx, params)
	s.logToolCall(params.Name, start, result.IsError, err)
	return result, err
}

func (s *Server) dispatchTool(ctx context.Context, params callToolParams) (toolResult, error) {
	switch params.Name {
	case "get_namespace_health":
		return s.getNamespaceHealth(ctx, params.Argument)
	case "explain_resource":
		return s.explainResource(ctx, params.Argument)
	case "trace_service":
		return s.traceService(ctx, params.Argument)
	case "get_pod_graph":
		return s.getPodGraph(ctx, params.Argument)
	case "dump_namespace":
		return s.dumpNamespace(ctx, params.Argument)
	default:
		return toolResult{}, fmt.Errorf("unknown tool %q", params.Name)
	}
}

func (s *Server) logToolCall(name string, start time.Time, isToolError bool, err error) {
	if s.logger == nil {
		return
	}
	attrs := []any{
		slog.String("tool", name),
		slog.Duration("duration", time.Since(start)),
		slog.Bool("tool_error", isToolError),
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
		s.logger.Error("mcp tool call failed", attrs...)
		return
	}
	s.logger.Info("mcp tool call completed", attrs...)
}

func (s *Server) toolContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx := parent
	cancel := func() {}
	if s.requestTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, s.requestTimeout)
	}
	if s.kubeAPIBudget > 0 {
		ctx = limits.ContextWithBudget(ctx, limits.NewBudget(s.kubeAPIBudget))
	}
	return ctx, cancel
}

type namespaceArgs struct {
	Namespace string `json:"namespace"`
}

func (s *Server) getNamespaceHealth(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args namespaceArgs
	if err := decodeArgs(raw, &args); err != nil {
		return badRequestToolResult(err.Error()), nil
	}
	if args.Namespace == "" {
		return badRequestToolResult("namespace is required"), nil
	}
	result, err := s.engine.NamespaceHealth(ctx, engine.NamespaceHealthRequest{Namespace: args.Namespace})
	if err != nil {
		return errorToolResult(err), nil
	}
	return successToolResult(result)
}

type explainArgs struct {
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	EventLimit int    `json:"eventLimit,omitempty"`
}

func (s *Server) explainResource(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args explainArgs
	if err := decodeArgs(raw, &args); err != nil {
		return badRequestToolResult(err.Error()), nil
	}
	if args.Resource == "" {
		return badRequestToolResult("resource is required"), nil
	}
	if args.Name == "" {
		return badRequestToolResult("name is required"), nil
	}
	if args.EventLimit > maxEventLimit {
		return badRequestToolResult(fmt.Sprintf("eventLimit must be less than or equal to %d", maxEventLimit)), nil
	}
	result, err := s.engine.ResolveContext(ctx, engine.ResolveContextRequest{
		Resource:   args.Resource,
		Namespace:  args.Namespace,
		Name:       args.Name,
		EventLimit: args.EventLimit,
	})
	if err != nil {
		return errorToolResult(err), nil
	}
	return successToolResult(result)
}

type traceServiceArgs struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func (s *Server) traceService(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args traceServiceArgs
	if err := decodeArgs(raw, &args); err != nil {
		return badRequestToolResult(err.Error()), nil
	}
	if args.Namespace == "" {
		return badRequestToolResult("namespace is required"), nil
	}
	if args.Name == "" {
		return badRequestToolResult("name is required"), nil
	}
	result, err := s.engine.TraceService(ctx, engine.TraceServiceRequest{Namespace: args.Namespace, Name: args.Name})
	if err != nil {
		return errorToolResult(err), nil
	}
	return successToolResult(result)
}

type podGraphArgs struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func (s *Server) getPodGraph(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args podGraphArgs
	if err := decodeArgs(raw, &args); err != nil {
		return badRequestToolResult(err.Error()), nil
	}
	if args.Namespace == "" {
		return badRequestToolResult("namespace is required"), nil
	}
	if args.Name == "" {
		return badRequestToolResult("name is required"), nil
	}
	result, err := s.engine.BuildPodGraph(ctx, engine.BuildPodGraphRequest{Namespace: args.Namespace, Name: args.Name})
	if err != nil {
		return errorToolResult(err), nil
	}
	return successToolResult(result)
}

func (s *Server) dumpNamespace(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args namespaceArgs
	if err := decodeArgs(raw, &args); err != nil {
		return badRequestToolResult(err.Error()), nil
	}
	if args.Namespace == "" {
		return badRequestToolResult("namespace is required"), nil
	}
	result, err := s.engine.DumpNamespace(ctx, engine.DumpNamespaceRequest{Namespace: args.Namespace})
	if err != nil {
		return errorToolResult(err), nil
	}
	return successToolResult(result)
}

func decodeArgs(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("invalid tool arguments")
	}
	return nil
}
