package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/observability"
	"github.com/lucasepe/kctx/internal/server/use"
)

var (
	errBadRequest       = errors.New("bad request")
	errNotFound         = errors.New("not found")
	errMethodNotAllowed = errors.New("method not allowed")
)

type Server struct {
	engine         *engine.Engine
	mux            *http.ServeMux
	logger         *slog.Logger
	metrics        *observability.ServerMetrics
	requestTimeout time.Duration
	kubeAPIBudget  int
}

const DefaultRequestTimeout = 30 * time.Second
const DefaultKubeAPIBudget = 100

type Option func(*Server)

// WithRequestTimeout sets the application-level deadline applied to each HTTP
// request. The network server timeouts still protect sockets; this protects the
// work performed by handlers and Kubernetes clients.
func WithRequestTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		s.requestTimeout = timeout
	}
}

// WithKubeAPIBudget limits how many Kubernetes client operations one HTTP
// request may perform. A value of 0 disables the guard for development or
// explicitly trusted deployments.
func WithKubeAPIBudget(limit int) Option {
	return func(s *Server) {
		s.kubeAPIBudget = limit
	}
}

func New(eng *engine.Engine, logger *slog.Logger, opts ...Option) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		engine:         eng,
		mux:            http.NewServeMux(),
		logger:         logger,
		metrics:        observability.NewServerMetrics(),
		requestTimeout: DefaultRequestTimeout,
		kubeAPIBudget:  DefaultKubeAPIBudget,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(s)
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return use.NewChain(
		use.RequestID(),
		use.RequestTimeout(s.requestTimeout),
		use.KubeAPIBudget(s.kubeAPIBudget),
		use.Logger(s.logger),
		use.Access(s.logger, s.metrics),
	).Then(s.mux)
}

func (s *Server) ListenAndServe(ctx context.Context, listen string) error {
	server := &http.Server{
		Addr:              listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error while server shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
