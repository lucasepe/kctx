package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/server/use"
)

var (
	errBadRequest       = errors.New("bad request")
	errNotFound         = errors.New("not found")
	errMethodNotAllowed = errors.New("method not allowed")
)

type Server struct {
	engine *engine.Engine
	mux    *http.ServeMux
	logger *slog.Logger
}

func New(eng *engine.Engine, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{engine: eng, mux: http.NewServeMux(), logger: logger}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return use.NewChain(
		use.Logger(s.logger),
		use.Access(s.logger),
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
