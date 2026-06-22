package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	workqueue "github.com/lucasepe/kctx/internal/queue"
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
		s.handleStreamableGet(w, r)
	case http.MethodDelete:
		s.handleStreamableDelete(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleStreamableGet replays retained Streamable HTTP events after Last-Event-ID.
func (s *HTTPServer) handleStreamableGet(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(mcpSessionIDHeader)
	if sessionID == "" || !s.validStreamableSession(r) {
		http.Error(w, "missing or expired MCP session", http.StatusBadRequest)
		return
	}
	lastEventID := strings.TrimSpace(r.Header.Get(lastEventIDHeader))
	if lastEventID == "" {
		http.Error(w, "missing Last-Event-ID", http.StatusBadRequest)
		return
	}
	streamID, ok := streamIDFromEventID(lastEventID)
	if !ok {
		http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
		return
	}
	if _, ok := s.streams.Get(r.Context(), sessionID, streamID); !ok {
		http.Error(w, "missing or expired MCP stream", http.StatusNotFound)
		return
	}

	events, err := s.streamEvents.After(r.Context(), sessionID, streamID, lastEventID)
	if err != nil {
		if errors.Is(err, errStreamEventNotFound) {
			http.Error(w, "missing or expired MCP event cursor", http.StatusNotFound)
			return
		}
		http.Error(w, "cannot replay MCP stream events", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	for _, event := range events {
		if err := writeSSEEvent(w, event); err != nil {
			if s.logger != nil {
				s.logger.Warn("mcp stream event replay failed", slog.String("err", err.Error()))
			}
			return
		}
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
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

	stream := s.createResponseStream(r, meta)
	if stream.ID != "" && acceptsEventStream(r) {
		s.writeStreamableResponse(w, r, stream, req)
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
	if err := s.writeLimitedJSON(w, resp); err != nil {
		if s.logger != nil {
			s.logger.Warn("mcp http response rejected", slog.String("err", err.Error()))
		}
		return
	}
	s.recordResponseEvent(r, stream, resp)
}

type streamableResponseResult struct {
	event StreamEvent
	err   error
}

// writeStreamableResponse enqueues a session-bound request and writes its final
// JSON-RPC response as one SSE event when the client remains connected.
func (s *HTTPServer) writeStreamableResponse(w http.ResponseWriter, r *http.Request, stream Stream, req json.RawMessage) {
	resultCh := make(chan streamableResponseResult, 1)
	job := workqueue.NewJob(nil, func(ctx context.Context, _ interface{}) {
		resultCh <- s.runStreamableResponseJob(ctx, stream, req)
	})
	if err := s.requestQueue.Push(r.Context(), job); err != nil {
		s.writeStreamableQueueError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			if s.logger != nil {
				s.logger.Warn("mcp stream response failed", slog.String("err", result.err.Error()))
			}
			return
		}
		if err := writeSSEEvent(w, result.event); err != nil && s.logger != nil {
			s.logger.Warn("mcp stream response failed", slog.String("err", err.Error()))
		}
	case <-r.Context().Done():
		return
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// runStreamableResponseJob performs the MCP work after the request has been
// accepted by the background queue.
func (s *HTTPServer) runStreamableResponseJob(ctx context.Context, stream Stream, req json.RawMessage) streamableResponseResult {
	resp, respond := s.server.handleMessage(ctx, req)
	if !respond {
		return streamableResponseResult{err: errors.New("streamable request did not produce a response")}
	}

	data, err := s.marshalLimitedJSON(resp)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("mcp http response rejected", slog.String("err", err.Error()))
		}
		data = s.streamableJSONLimitErrorData(resp.ID, err)
	}
	event := s.recordResponseEventData(ctx, stream, data)
	return streamableResponseResult{event: event}
}

// streamableJSONLimitErrorData builds a JSON-RPC error event for responses that
// cannot be sent as-is because they fail encoding or response-size checks.
func (s *HTTPServer) streamableJSONLimitErrorData(id json.RawMessage, err error) []byte {
	if strings.HasPrefix(err.Error(), "encode MCP response") {
		data, _ := json.Marshal(errorResponse(id, invalidRequest, "encode MCP response", nil))
		return data
	}

	data, _ := json.Marshal(errorResponse(id, invalidRequest, "MCP response exceeds max response bytes", map[string]string{
		"maxBytes": fmt.Sprintf("%d", s.maxResponseBytes),
	}))
	return data
}

// writeStreamableQueueError maps queue admission failures to HTTP responses.
func (s *HTTPServer) writeStreamableQueueError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workqueue.ErrQueueFull):
		http.Error(w, "MCP request queue is full", http.StatusTooManyRequests)
	case errors.Is(err, workqueue.ErrQueueClosed), errors.Is(err, workqueue.ErrQueueNotRunning):
		http.Error(w, "MCP request queue is unavailable", http.StatusServiceUnavailable)
	case errors.Is(err, context.Canceled):
		http.Error(w, "MCP request canceled", http.StatusRequestTimeout)
	case errors.Is(err, context.DeadlineExceeded):
		http.Error(w, "MCP request timed out", http.StatusRequestTimeout)
	default:
		http.Error(w, "MCP request queue rejected the job", http.StatusInternalServerError)
	}
	if s.logger != nil {
		s.logger.Warn("mcp stream request not queued", slog.String("err", err.Error()))
	}
}

// createResponseStream records the logical stream for one session-bound request.
func (s *HTTPServer) createResponseStream(r *http.Request, meta streamableMessageMeta) Stream {
	if meta.isInitialize() || len(meta.ID) == 0 {
		return Stream{}
	}
	sessionID := r.Header.Get(mcpSessionIDHeader)
	requestID := requestIDFromRaw(meta.ID)
	if sessionID == "" || requestID == "" {
		return Stream{}
	}

	stream, err := s.streams.Create(r.Context(), sessionID, requestID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("mcp stream not recorded", slog.String("err", err.Error()))
		}
		return Stream{}
	}
	if _, err := s.streamRequests.Create(r.Context(), sessionID, stream.ID, requestID); err != nil && s.logger != nil {
		s.logger.Warn("mcp stream request not recorded", slog.String("err", err.Error()))
	}
	return stream
}

// recordResponseEvent stores the final JSON-RPC response for future replay.
func (s *HTTPServer) recordResponseEvent(r *http.Request, stream Stream, resp response) {
	if stream.ID == "" {
		return
	}
	data, err := json.Marshal(resp)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("mcp stream response not encoded", slog.String("err", err.Error()))
		}
		return
	}
	s.recordResponseEventData(r.Context(), stream, data)
}

// recordResponseEventData stores response data and marks its stream completed.
func (s *HTTPServer) recordResponseEventData(ctx context.Context, stream Stream, data []byte) StreamEvent {
	event := StreamEvent{
		ID:        s.streamEventIDs.Next(stream.ID),
		SessionID: stream.SessionID,
		StreamID:  stream.ID,
		RequestID: stream.RequestID,
		Data:      data,
	}
	if err := s.streamEvents.Append(ctx, event); err != nil && s.logger != nil {
		s.logger.Warn("mcp stream event not recorded", slog.String("err", err.Error()))
	}
	if _, err := s.streams.Complete(ctx, stream.SessionID, stream.ID); err != nil && s.logger != nil {
		s.logger.Warn("mcp stream not completed", slog.String("err", err.Error()))
	}
	if _, err := s.streamRequests.Complete(ctx, stream.SessionID, stream.ID); err != nil && s.logger != nil {
		s.logger.Warn("mcp stream request not completed", slog.String("err", err.Error()))
	}
	return event
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
	if err := s.streams.DeleteSession(r.Context(), sessionID); err != nil && s.logger != nil {
		s.logger.Warn("mcp streams not deleted", slog.String("err", err.Error()))
	}
	if err := s.streamEvents.DeleteSession(r.Context(), sessionID); err != nil && s.logger != nil {
		s.logger.Warn("mcp stream events not deleted", slog.String("err", err.Error()))
	}
	if err := s.streamRequests.DeleteSession(r.Context(), sessionID); err != nil && s.logger != nil {
		s.logger.Warn("mcp stream requests not deleted", slog.String("err", err.Error()))
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
	ID     json.RawMessage `json:"id"`
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

// requestIDFromRaw returns the compact request id used for stream deduplication.
func requestIDFromRaw(raw json.RawMessage) string {
	return strings.TrimSpace(string(raw))
}

// acceptsEventStream reports whether the client prefers an SSE response.
func acceptsEventStream(r *http.Request) bool {
	jsonSeen := false
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		mediaType := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if mediaType == "application/json" {
			jsonSeen = true
			continue
		}
		if mediaType == "text/event-stream" {
			return !jsonSeen
		}
	}
	return false
}
