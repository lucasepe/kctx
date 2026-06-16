package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/limits"
)

type callToolParams struct {
	Name     string          `json:"name"`
	Argument json.RawMessage `json:"arguments"`
}

// callTool validates a tools/call request and dispatches it with server limits.
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

// dispatchTool maps MCP tool names to kctx engine operations.
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

// logToolCall writes one structured log line for a completed tool call.
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

// toolContext applies the request timeout and Kubernetes API budget to a call.
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

// getNamespaceHealth returns a compact namespace health snapshot.
func (s *Server) getNamespaceHealth(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args namespaceArgs
	if err := decodeArgs(raw, &args); err != nil {
		return s.badRequestToolResult(err.Error()), nil
	}
	if args.Namespace == "" {
		return s.badRequestToolResult("namespace is required"), nil
	}
	result, err := s.engine.NamespaceHealth(ctx, engine.NamespaceHealthRequest{Namespace: args.Namespace})
	if err != nil {
		return s.errorToolResult(err), nil
	}
	return s.successToolResult(result)
}

type explainArgs struct {
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	EventLimit int    `json:"eventLimit,omitempty"`
}

// explainResource returns normalized context for one Kubernetes resource.
func (s *Server) explainResource(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args explainArgs
	if err := decodeArgs(raw, &args); err != nil {
		return s.badRequestToolResult(err.Error()), nil
	}
	if args.Resource == "" {
		return s.badRequestToolResult("resource is required"), nil
	}
	if args.Name == "" {
		return s.badRequestToolResult("name is required"), nil
	}
	if args.EventLimit > maxEventLimit {
		return s.badRequestToolResult(fmt.Sprintf("eventLimit must be less than or equal to %d", maxEventLimit)), nil
	}
	result, err := s.engine.ResolveContext(ctx, engine.ResolveContextRequest{
		Resource:   args.Resource,
		Namespace:  args.Namespace,
		Name:       args.Name,
		EventLimit: args.EventLimit,
	})
	if err != nil {
		return s.errorToolResult(err), nil
	}
	return s.successToolResult(result)
}

type traceServiceArgs struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// traceService traces a Service to endpoints, pods, owners, and related signals.
func (s *Server) traceService(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args traceServiceArgs
	if err := decodeArgs(raw, &args); err != nil {
		return s.badRequestToolResult(err.Error()), nil
	}
	if args.Namespace == "" {
		return s.badRequestToolResult("namespace is required"), nil
	}
	if args.Name == "" {
		return s.badRequestToolResult("name is required"), nil
	}
	result, err := s.engine.TraceService(ctx, engine.TraceServiceRequest{Namespace: args.Namespace, Name: args.Name})
	if err != nil {
		return s.errorToolResult(err), nil
	}
	return s.successToolResult(result)
}

type podGraphArgs struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// getPodGraph returns the ownership and scheduling graph around one Pod.
func (s *Server) getPodGraph(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args podGraphArgs
	if err := decodeArgs(raw, &args); err != nil {
		return s.badRequestToolResult(err.Error()), nil
	}
	if args.Namespace == "" {
		return s.badRequestToolResult("namespace is required"), nil
	}
	if args.Name == "" {
		return s.badRequestToolResult("name is required"), nil
	}
	result, err := s.engine.BuildPodGraph(ctx, engine.BuildPodGraphRequest{Namespace: args.Namespace, Name: args.Name})
	if err != nil {
		return s.errorToolResult(err), nil
	}
	return s.successToolResult(result)
}

// dumpNamespace returns the namespace inventory used by heavier diagnostics.
func (s *Server) dumpNamespace(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args namespaceArgs
	if err := decodeArgs(raw, &args); err != nil {
		return s.badRequestToolResult(err.Error()), nil
	}
	if args.Namespace == "" {
		return s.badRequestToolResult("namespace is required"), nil
	}
	result, err := s.engine.DumpNamespace(ctx, engine.DumpNamespaceRequest{Namespace: args.Namespace})
	if err != nil {
		return s.errorToolResult(err), nil
	}
	return s.successToolResult(result)
}

// decodeArgs unmarshals tool arguments, treating omitted params as an empty object.
func decodeArgs(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("invalid tool arguments")
	}
	return nil
}
