package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const (
	sseEndpointPath = "/mcp/sse"
	sseMessagePath  = "/mcp/message"
)

// SSEServer exposes the MCP server over the legacy HTTP/SSE transport. Clients
// open an SSE stream, receive a per-session POST endpoint, then send JSON-RPC
// messages to that endpoint and receive responses on the stream.
type SSEServer struct {
	server   *Server
	logger   *slog.Logger
	sessions *sessionStore
}

// NewSSEServer creates an HTTP/SSE transport for an MCP protocol server.
func NewSSEServer(server *Server, logger *slog.Logger) *SSEServer {
	if logger == nil {
		logger = slog.Default()
	}
	return &SSEServer{
		server:   server,
		logger:   logger,
		sessions: newSessionStore(),
	}
}

// Handler returns an HTTP handler exposing MCP SSE endpoints plus small health
// endpoints so the same Kubernetes probes used by kctx serve can stay enabled.
func (s *SSEServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", s.livez)
	mux.HandleFunc("/readyz", s.readyz)
	mux.HandleFunc("/version", s.version)
	mux.HandleFunc(sseEndpointPath, s.handleSSE)
	mux.HandleFunc(sseMessagePath, s.handleMessage)
	return mux
}

// ListenAndServe starts the HTTP/SSE server and shuts it down when ctx is
// canceled. WriteTimeout is intentionally disabled because SSE streams are
// long-lived responses.
func (s *SSEServer) ListenAndServe(ctx context.Context, listen string) error {
	httpServer := &http.Server{
		Addr:              listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
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
			return fmt.Errorf("shutdown MCP SSE server: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *SSEServer) livez(w http.ResponseWriter, r *http.Request) {
	writeHTTPJSON(w, map[string]string{"status": "alive"})
}

func (s *SSEServer) readyz(w http.ResponseWriter, r *http.Request) {
	if s.server == nil || s.server.engine == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = writeHTTPJSON(w, map[string]string{"status": "not_ready"})
		return
	}
	writeHTTPJSON(w, map[string]string{"status": "ready"})
}

func (s *SSEServer) version(w http.ResponseWriter, r *http.Request) {
	version := defaultVersion
	if s.server != nil && s.server.version != "" {
		version = s.server.version
	}
	writeHTTPJSON(w, map[string]string{"name": "kctx", "version": version})
}

func (s *SSEServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "only GET is supported", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE streaming is not supported", http.StatusInternalServerError)
		return
	}

	sessionID, err := newSessionID()
	if err != nil {
		http.Error(w, "cannot create MCP session", http.StatusInternalServerError)
		return
	}
	session := s.sessions.add(r.Context(), sessionID)
	defer s.sessions.remove(sessionID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	endpoint := sseMessagePath + "?sessionId=" + sessionID
	if err := writeSSE(w, flusher, "endpoint", endpoint); err != nil {
		return
	}

	keepAlive := time.NewTicker(30 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case msg := <-session.messages:
			if err := writeSSE(w, flusher, "message", string(msg)); err != nil {
				return
			}
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *SSEServer) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "only POST is supported", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	session, ok := s.sessions.get(sessionID)
	if !ok {
		http.Error(w, "invalid or expired MCP session", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp := errorResponse(nil, parseError, "invalid JSON-RPC message", nil)
		session.enqueue(resp)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Acknowledge the POST before running the tool. MCP SSE responses travel on
	// the long-lived stream, so slow Kubernetes reads should not keep the POST
	// request open or depend on the POST request context.
	w.WriteHeader(http.StatusAccepted)
	go s.dispatchSessionMessage(session, req)
}

func (s *SSEServer) dispatchSessionMessage(session *sseSession, req json.RawMessage) {
	resp, respond := s.server.handleMessage(session.ctx, req)
	if !respond {
		return
	}
	if err := session.enqueue(resp); err != nil && s.logger != nil {
		s.logger.Warn("mcp sse response dropped", slog.String("err", err.Error()))
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event, data string) error {
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeHTTPJSON(w http.ResponseWriter, value any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(value)
}

func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

type sseSession struct {
	ctx      context.Context
	cancel   context.CancelFunc
	messages chan []byte
}

func (s *sseSession) enqueue(resp response) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("encode MCP response: %w", err)
	}
	select {
	case s.messages <- data:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("MCP session is not consuming responses")
	}
}

type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*sseSession
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: map[string]*sseSession{}}
}

func (s *sessionStore) add(parent context.Context, id string) *sseSession {
	ctx, cancel := context.WithCancel(parent)
	session := &sseSession{ctx: ctx, cancel: cancel, messages: make(chan []byte, 16)}
	s.mu.Lock()
	s.sessions[id] = session
	s.mu.Unlock()
	return session
}

func (s *sessionStore) get(id string) (*sseSession, bool) {
	if id == "" {
		return nil, false
	}
	s.mu.RLock()
	session, ok := s.sessions[id]
	s.mu.RUnlock()
	return session, ok
}

func (s *sessionStore) remove(id string) {
	s.mu.Lock()
	session := s.sessions[id]
	if session != nil {
		session.cancel()
	}
	delete(s.sessions, id)
	s.mu.Unlock()
}
