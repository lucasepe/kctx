package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/lucasepe/kctx/internal/engine"
)

// maxEventLimit keeps a single Pod context request from asking for an
// unbounded event payload. The engine still has its own small default when the
// query parameter is omitted.
const maxEventLimit = 500

func (s *Server) livez(w http.ResponseWriter, r *http.Request) {
	s.handle(w, r, func(w http.ResponseWriter, r *http.Request) error {
		return writeJSON(w, map[string]string{"status": "alive"})
	})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	s.handle(w, r, func(w http.ResponseWriter, r *http.Request) error {
		if s.engine == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return writeJSON(w, map[string]string{"status": "not_ready"})
		}
		return writeJSON(w, map[string]string{"status": "ready"})
	})
}

func (s *Server) metricsHTTP(w http.ResponseWriter, r *http.Request) {
	s.handle(w, r, func(w http.ResponseWriter, r *http.Request) error {
		s.metrics.ServeHTTP(w, r)
		return nil
	})
}

func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	s.handle(w, r, func(w http.ResponseWriter, r *http.Request) error {
		return writeJSON(w, map[string]string{"name": "kctx", "version": "dev"})
	})
}

func (s *Server) contextPod(w http.ResponseWriter, r *http.Request) {
	s.handle(w, r, func(w http.ResponseWriter, r *http.Request) error {
		namespace, name, err := pathPair(r.URL.Path, "/context/pod/")
		if err != nil {
			return err
		}
		eventLimit, err := intQuery(r, "eventLimit", maxEventLimit)
		if err != nil {
			return err
		}
		result, err := s.engine.ResolvePodContext(r.Context(), engine.ResolvePodContextRequest{Namespace: namespace, Name: name, EventLimit: eventLimit})
		if err != nil {
			return err
		}
		return writeJSON(w, result)
	})
}

func (s *Server) graphPod(w http.ResponseWriter, r *http.Request) {
	s.handle(w, r, func(w http.ResponseWriter, r *http.Request) error {
		namespace, name, err := pathPair(r.URL.Path, "/graph/pod/")
		if err != nil {
			return err
		}
		result, err := s.engine.BuildPodGraph(r.Context(), engine.BuildPodGraphRequest{Namespace: namespace, Name: name})
		if err != nil {
			return err
		}
		return writeJSON(w, result)
	})
}

func (s *Server) traceService(w http.ResponseWriter, r *http.Request) {
	s.handle(w, r, func(w http.ResponseWriter, r *http.Request) error {
		namespace, name, err := pathPair(r.URL.Path, "/trace/service/")
		if err != nil {
			return err
		}
		result, err := s.engine.TraceService(r.Context(), engine.TraceServiceRequest{Namespace: namespace, Name: name})
		if err != nil {
			return err
		}
		return writeJSON(w, result)
	})
}

func (s *Server) healthNamespace(w http.ResponseWriter, r *http.Request) {
	s.handle(w, r, func(w http.ResponseWriter, r *http.Request) error {
		namespace, err := pathSingle(r.URL.Path, "/health/namespace/")
		if err != nil {
			return err
		}
		result, err := s.engine.NamespaceHealth(r.Context(), engine.NamespaceHealthRequest{Namespace: namespace})
		if err != nil {
			return err
		}
		return writeJSON(w, result)
	})
}

func (s *Server) dumpNamespace(w http.ResponseWriter, r *http.Request) {
	s.handle(w, r, func(w http.ResponseWriter, r *http.Request) error {
		namespace, err := pathSingle(r.URL.Path, "/dump/namespace/")
		if err != nil {
			return err
		}
		result, err := s.engine.DumpNamespace(r.Context(), engine.DumpNamespaceRequest{Namespace: namespace})
		if err != nil {
			return err
		}
		return writeJSON(w, result)
	})
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request, fn func(http.ResponseWriter, *http.Request) error) {
	if r.Method != http.MethodGet {
		writeError(w, fmt.Errorf("%w: only GET is supported", errMethodNotAllowed))
		return
	}
	if err := fn(w, r); err != nil {
		writeError(w, err)
	}
}

func writeJSON(w http.ResponseWriter, value any) error {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status, body := errorResponse(err)
	if status == 499 {
		// Il client ha già chiuso la connessione. Inutile scrivere il JSON.
		// Puoi opzionalmente loggare l'evento internamente se usi un logger.
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func intQuery(r *http.Request, name string, max int) (int, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return 0, nil
	}
	out, err := strconv.Atoi(value)
	if err != nil || out < 0 {
		return 0, fmt.Errorf("%w: %s must be a non-negative integer", errBadRequest, name)
	}
	if max > 0 && out > max {
		return 0, fmt.Errorf("%w: %s must be less than or equal to %d", errBadRequest, name, max)
	}
	return out, nil
}

func pathPair(path, prefix string) (string, string, error) {
	rest := strings.TrimPrefix(path, prefix)
	if rest == path || rest == "" {
		return "", "", fmt.Errorf("%w: route not found", errNotFound)
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("%w: expected %s{namespace}/{name}", errNotFound, prefix)
	}
	return parts[0], parts[1], nil
}

func pathSingle(path, prefix string) (string, error) {
	rest := strings.TrimPrefix(path, prefix)
	if rest == path || rest == "" || strings.Contains(rest, "/") {
		return "", fmt.Errorf("%w: expected %s{namespace}", errNotFound, prefix)
	}
	return rest, nil
}
