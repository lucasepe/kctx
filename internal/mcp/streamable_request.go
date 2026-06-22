package mcp

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	errStreamRequestMissingFields    = errors.New("stream request requires session id, stream id, and request id")
	errStreamRequestNotFound         = errors.New("stream request not found")
	errStreamRequestStateTransition  = errors.New("stream request state transition is not allowed")
	errStreamRequestDuplicateRequest = errors.New("stream request id already exists")
)

// StreamRequestState is the lifecycle state of one Streamable HTTP request job.
type StreamRequestState string

const (
	StreamRequestRunning   StreamRequestState = "running"
	StreamRequestCompleted StreamRequestState = "completed"
	StreamRequestCanceled  StreamRequestState = "canceled"
	StreamRequestExpired   StreamRequestState = "expired"
)

// StreamRequest represents one JSON-RPC request tracked beyond one connection.
//
// The request registry prepares safe retry semantics: a future transport layer
// can detect whether a request is still running, already completed, or expired.
type StreamRequest struct {
	SessionID string
	StreamID  string
	RequestID string
	State     StreamRequestState
	StartedAt time.Time
	UpdatedAt time.Time
	ExpiresAt time.Time
}

// StreamRequestStore stores request jobs for Streamable HTTP sessions.
//
// Implementations may keep requests in memory, Redis, or another shared store.
// The transport only depends on this interface so request state can become
// persistent later without changing request handling.
type StreamRequestStore interface {
	Create(ctx context.Context, sessionID, streamID, requestID string) (StreamRequest, error)
	Get(ctx context.Context, sessionID, streamID string) (StreamRequest, bool)
	FindByRequest(ctx context.Context, sessionID, requestID string) (StreamRequest, bool)
	Complete(ctx context.Context, sessionID, streamID string) (StreamRequest, error)
	Cancel(ctx context.Context, sessionID, streamID string) (StreamRequest, error)
	Expire(ctx context.Context, sessionID, streamID string) (StreamRequest, error)
	DeleteStream(ctx context.Context, sessionID, streamID string) error
	DeleteSession(ctx context.Context, sessionID string) error
}

type memoryStreamRequestStore struct {
	mu       sync.RWMutex
	ttl      time.Duration
	now      func() time.Time
	requests map[string]map[string]StreamRequest
}

func newMemoryStreamRequestStore(ttl time.Duration) *memoryStreamRequestStore {
	if ttl <= 0 {
		ttl = streamableSessionTTL
	}
	return &memoryStreamRequestStore{
		ttl:      ttl,
		now:      time.Now,
		requests: map[string]map[string]StreamRequest{},
	}
}

func (s *memoryStreamRequestStore) Create(ctx context.Context, sessionID, streamID, requestID string) (StreamRequest, error) {
	select {
	case <-ctx.Done():
		return StreamRequest{}, ctx.Err()
	default:
	}
	if sessionID == "" || streamID == "" || requestID == "" {
		return StreamRequest{}, errStreamRequestMissingFields
	}

	now := s.now()
	req := StreamRequest{
		SessionID: sessionID,
		StreamID:  streamID,
		RequestID: requestID,
		State:     StreamRequestRunning,
		StartedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}

	s.mu.Lock()
	s.pruneExpiredLocked(now)
	requests := s.requests[sessionID]
	if requests == nil {
		requests = map[string]StreamRequest{}
		s.requests[sessionID] = requests
	}
	for _, existing := range requests {
		if existing.RequestID == requestID {
			s.mu.Unlock()
			return StreamRequest{}, errStreamRequestDuplicateRequest
		}
	}
	requests[streamID] = req
	s.mu.Unlock()
	return req, nil
}

func (s *memoryStreamRequestStore) Get(ctx context.Context, sessionID, streamID string) (StreamRequest, bool) {
	select {
	case <-ctx.Done():
		return StreamRequest{}, false
	default:
	}
	if sessionID == "" || streamID == "" {
		return StreamRequest{}, false
	}

	now := s.now()
	s.mu.Lock()
	s.pruneExpiredLocked(now)
	req, ok := s.requests[sessionID][streamID]
	s.mu.Unlock()
	return req, ok
}

func (s *memoryStreamRequestStore) FindByRequest(ctx context.Context, sessionID, requestID string) (StreamRequest, bool) {
	select {
	case <-ctx.Done():
		return StreamRequest{}, false
	default:
	}
	if sessionID == "" || requestID == "" {
		return StreamRequest{}, false
	}

	now := s.now()
	s.mu.Lock()
	s.pruneExpiredLocked(now)
	requests := s.requests[sessionID]
	for _, req := range requests {
		if req.RequestID == requestID {
			s.mu.Unlock()
			return req, true
		}
	}
	s.mu.Unlock()
	return StreamRequest{}, false
}

func (s *memoryStreamRequestStore) Complete(ctx context.Context, sessionID, streamID string) (StreamRequest, error) {
	return s.setState(ctx, sessionID, streamID, StreamRequestCompleted)
}

func (s *memoryStreamRequestStore) Cancel(ctx context.Context, sessionID, streamID string) (StreamRequest, error) {
	return s.setState(ctx, sessionID, streamID, StreamRequestCanceled)
}

func (s *memoryStreamRequestStore) Expire(ctx context.Context, sessionID, streamID string) (StreamRequest, error) {
	return s.setState(ctx, sessionID, streamID, StreamRequestExpired)
}

func (s *memoryStreamRequestStore) DeleteStream(ctx context.Context, sessionID, streamID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	if requests := s.requests[sessionID]; requests != nil {
		delete(requests, streamID)
		if len(requests) == 0 {
			delete(s.requests, sessionID)
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *memoryStreamRequestStore) DeleteSession(ctx context.Context, sessionID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	delete(s.requests, sessionID)
	s.mu.Unlock()
	return nil
}

func (s *memoryStreamRequestStore) setState(ctx context.Context, sessionID, streamID string, state StreamRequestState) (StreamRequest, error) {
	select {
	case <-ctx.Done():
		return StreamRequest{}, ctx.Err()
	default:
	}
	if sessionID == "" || streamID == "" {
		return StreamRequest{}, errStreamRequestMissingFields
	}

	now := s.now()
	s.mu.Lock()
	s.pruneExpiredLocked(now)
	req, ok := s.requests[sessionID][streamID]
	if !ok {
		s.mu.Unlock()
		return StreamRequest{}, errStreamRequestNotFound
	}
	if req.State != StreamRequestRunning && req.State != state {
		s.mu.Unlock()
		return StreamRequest{}, errStreamRequestStateTransition
	}
	if req.State == state {
		s.mu.Unlock()
		return req, nil
	}
	req.State = state
	req.UpdatedAt = now
	req.ExpiresAt = now.Add(s.ttl)
	s.requests[sessionID][streamID] = req
	s.mu.Unlock()
	return req, nil
}

func (s *memoryStreamRequestStore) pruneExpiredLocked(now time.Time) {
	for sessionID, requests := range s.requests {
		for streamID, req := range requests {
			if !now.Before(req.ExpiresAt) {
				delete(requests, streamID)
			}
		}
		if len(requests) == 0 {
			delete(s.requests, sessionID)
		}
	}
}
