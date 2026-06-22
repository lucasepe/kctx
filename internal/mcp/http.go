package mcp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	workqueue "github.com/lucasepe/kctx/internal/queue"
)

const (
	streamableEndpointPath  = "/mcp"
	mcpSessionIDHeader      = "Mcp-Session-Id"
	mcpProtocolHeader       = "MCP-Protocol-Version"
	lastEventIDHeader       = "Last-Event-ID"
	corsAllowHeaders        = "Accept, Authorization, Content-Type, Last-Event-ID, Mcp-Session-Id, MCP-Protocol-Version"
	corsExposeHeaders       = "Mcp-Session-Id, MCP-Protocol-Version"
	streamableSessionTTL    = 30 * time.Minute
	defaultMaxRequestBytes  = 1 << 20
	defaultMaxResponseBytes = 16 << 20
)

// HTTPServer exposes the MCP server over Streamable HTTP at /mcp.
type HTTPServer struct {
	server             *Server
	logger             *slog.Logger
	streamableSessions StreamableSessionStore
	streams            StreamStore
	streamEvents       StreamEventStore
	streamEventIDs     StreamEventIDGenerator
	streamRequests     StreamRequestStore
	requestQueue       *workqueue.Queue
	maxRequestBytes    int64
	maxResponseBytes   int64
	writeTimeout       time.Duration
}

// HTTPOption configures the MCP Streamable HTTP transport.
type HTTPOption func(*HTTPServer)

// WithMaxRequestBytes limits one incoming Streamable HTTP JSON-RPC request.
func WithMaxRequestBytes(limit int64) HTTPOption {
	return func(s *HTTPServer) {
		s.maxRequestBytes = limit
	}
}

// WithMaxResponseBytes limits one outgoing Streamable HTTP JSON-RPC response.
func WithMaxResponseBytes(limit int64) HTTPOption {
	return func(s *HTTPServer) {
		s.maxResponseBytes = limit
	}
}

// WithHTTPWriteTimeout sets the net/http write timeout for MCP HTTP responses.
func WithHTTPWriteTimeout(timeout time.Duration) HTTPOption {
	return func(s *HTTPServer) {
		s.writeTimeout = timeout
	}
}

// WithRequestQueue replaces the queue used for background Streamable HTTP jobs.
func WithRequestQueue(q *workqueue.Queue) HTTPOption {
	return func(s *HTTPServer) {
		if q != nil {
			s.requestQueue = q
		}
	}
}

// NewHTTPServer creates a Streamable HTTP transport for an MCP protocol server.
func NewHTTPServer(server *Server, logger *slog.Logger, opts ...HTTPOption) *HTTPServer {
	if logger == nil {
		logger = slog.Default()
	}
	s := &HTTPServer{
		server:             server,
		logger:             logger,
		streamableSessions: newMemoryStreamableSessionStore(streamableSessionTTL),
		streams:            newMemoryStreamStore(streamableSessionTTL),
		streamEvents:       newMemoryStreamEventStore(streamableSessionTTL),
		streamEventIDs:     newMemoryStreamEventIDGenerator(),
		streamRequests:     newMemoryStreamRequestStore(streamableSessionTTL),
		requestQueue:       workqueue.New(64, 4),
		maxRequestBytes:    defaultMaxRequestBytes,
		maxResponseBytes:   defaultMaxResponseBytes,
		writeTimeout:       defaultHTTPWriteTimeout(server),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	s.requestQueue.Run()
	return s
}

// Handler returns an HTTP handler exposing the MCP Streamable HTTP endpoint
// plus small health endpoints so Kubernetes probes can stay enabled.
func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", s.livez)
	mux.HandleFunc("/readyz", s.readyz)
	mux.HandleFunc("/version", s.version)
	mux.HandleFunc(streamableEndpointPath, s.handleStreamableHTTP)
	return mux
}

// ListenAndServe starts the MCP HTTP server and shuts it down when ctx is canceled.
func (s *HTTPServer) ListenAndServe(ctx context.Context, listen string) error {
	defer s.requestQueue.Terminate()
	httpServer := &http.Server{
		Addr:              listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      s.writeTimeout,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return errors.Join(errShutdownHTTPServer, err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

var errShutdownHTTPServer = errors.New("shutdown MCP HTTP server")

// defaultHTTPWriteTimeout derives the HTTP write timeout from the tool timeout.
func defaultHTTPWriteTimeout(server *Server) time.Duration {
	if server == nil || server.requestTimeout <= 0 {
		return 0
	}
	return server.requestTimeout + 10*time.Second
}
