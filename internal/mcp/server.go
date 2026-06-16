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
)

const (
	protocolVersion                  = "2025-06-18"
	defaultVersion                   = "dev"
	maxEventLimit                    = 500
	defaultMaxToolResultBytes        = 8 << 20
	defaultStructuredContentMaxBytes = 1 << 20

	parseError     = -32700
	invalidRequest = -32600
	methodNotFound = -32601
	invalidParams  = -32602
)

// Server serves kctx tools over newline-delimited JSON-RPC 2.0 messages on
// stdio. The implementation intentionally supports the small MCP surface kctx
// needs first: initialization, tool listing, and tool calls.
type Server struct {
	engine                    *engine.Engine
	logger                    *slog.Logger
	version                   string
	requestTimeout            time.Duration
	kubeAPIBudget             int
	maxToolResultBytes        int64
	structuredContentMaxBytes int64
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

// WithMaxToolResultBytes limits the compact JSON payload embedded in one MCP
// tool result. A zero or negative value disables the guard.
func WithMaxToolResultBytes(limit int64) Option {
	return func(s *Server) {
		s.maxToolResultBytes = limit
	}
}

// WithStructuredContentMaxBytes omits structuredContent for larger payloads to
// avoid duplicating large namespace dumps in MCP responses.
func WithStructuredContentMaxBytes(limit int64) Option {
	return func(s *Server) {
		s.structuredContentMaxBytes = limit
	}
}

// New creates an MCP server backed by eng.
func New(eng *engine.Engine, logger *slog.Logger, opts ...Option) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		engine:                    eng,
		logger:                    logger,
		version:                   defaultVersion,
		requestTimeout:            30 * time.Second,
		kubeAPIBudget:             100,
		maxToolResultBytes:        defaultMaxToolResultBytes,
		structuredContentMaxBytes: defaultStructuredContentMaxBytes,
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
