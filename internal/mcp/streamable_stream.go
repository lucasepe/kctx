package mcp

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	errStreamMissingFields     = errors.New("stream requires session id and stream id")
	errStreamNotFound          = errors.New("stream not found")
	errStreamSessionRequired   = errors.New("stream requires session id")
	errStreamStateTransition   = errors.New("stream state transition is not allowed")
	errStreamRequestDuplicate  = errors.New("stream request id already exists")
	errStreamRequestIDRequired = errors.New("stream request id is required")
)

// StreamState is the lifecycle state of one logical Streamable HTTP stream.
type StreamState string

const (
	StreamOpen      StreamState = "open"
	StreamCompleted StreamState = "completed"
	StreamCanceled  StreamState = "canceled"
	StreamExpired   StreamState = "expired"
)

// Stream represents one logical MCP Streamable HTTP stream.
//
// A Stream outlives any single HTTP connection. This lets the transport keep
// enough state to resume event delivery after a client disconnects.
type Stream struct {
	ID        string
	SessionID string
	RequestID string
	State     StreamState
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt time.Time
}

// StreamStore stores logical Streamable HTTP streams.
//
// Implementations may keep streams in memory, Redis, or another shared store.
// The transport only depends on this interface so stream state can become
// persistent later without changing request handling.
type StreamStore interface {
	Create(ctx context.Context, sessionID, requestID string) (Stream, error)
	Get(ctx context.Context, sessionID, streamID string) (Stream, bool)
	FindByRequest(ctx context.Context, sessionID, requestID string) (Stream, bool)
	Complete(ctx context.Context, sessionID, streamID string) (Stream, error)
	Cancel(ctx context.Context, sessionID, streamID string) (Stream, error)
	Expire(ctx context.Context, sessionID, streamID string) (Stream, error)
	DeleteStream(ctx context.Context, sessionID, streamID string) error
	DeleteSession(ctx context.Context, sessionID string) error
}

type memoryStreamStore struct {
	mu      sync.RWMutex
	ttl     time.Duration
	now     func() time.Time
	streams map[string]map[string]Stream
}

func newMemoryStreamStore(ttl time.Duration) *memoryStreamStore {
	if ttl <= 0 {
		ttl = streamableSessionTTL
	}
	return &memoryStreamStore{
		ttl:     ttl,
		now:     time.Now,
		streams: map[string]map[string]Stream{},
	}
}

func (s *memoryStreamStore) Create(ctx context.Context, sessionID, requestID string) (Stream, error) {
	select {
	case <-ctx.Done():
		return Stream{}, ctx.Err()
	default:
	}
	if sessionID == "" {
		return Stream{}, errStreamSessionRequired
	}
	if requestID == "" {
		return Stream{}, errStreamRequestIDRequired
	}

	now := s.now()
	streamID, err := newSessionID()
	if err != nil {
		return Stream{}, err
	}
	stream := Stream{
		ID:        streamID,
		SessionID: sessionID,
		RequestID: requestID,
		State:     StreamOpen,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}

	s.mu.Lock()
	s.pruneExpiredLocked(now)
	streams := s.streams[sessionID]
	if streams == nil {
		streams = map[string]Stream{}
		s.streams[sessionID] = streams
	}
	for _, existing := range streams {
		if existing.RequestID == requestID {
			s.mu.Unlock()
			return Stream{}, errStreamRequestDuplicate
		}
	}
	streams[stream.ID] = stream
	s.mu.Unlock()
	return stream, nil
}

func (s *memoryStreamStore) Get(ctx context.Context, sessionID, streamID string) (Stream, bool) {
	select {
	case <-ctx.Done():
		return Stream{}, false
	default:
	}
	if sessionID == "" || streamID == "" {
		return Stream{}, false
	}

	now := s.now()
	s.mu.Lock()
	s.pruneExpiredLocked(now)
	stream, ok := s.streams[sessionID][streamID]
	s.mu.Unlock()
	return stream, ok
}

func (s *memoryStreamStore) FindByRequest(ctx context.Context, sessionID, requestID string) (Stream, bool) {
	select {
	case <-ctx.Done():
		return Stream{}, false
	default:
	}
	if sessionID == "" || requestID == "" {
		return Stream{}, false
	}

	now := s.now()
	s.mu.Lock()
	s.pruneExpiredLocked(now)
	streams := s.streams[sessionID]
	for _, stream := range streams {
		if stream.RequestID == requestID {
			s.mu.Unlock()
			return stream, true
		}
	}
	s.mu.Unlock()
	return Stream{}, false
}

func (s *memoryStreamStore) Complete(ctx context.Context, sessionID, streamID string) (Stream, error) {
	return s.setState(ctx, sessionID, streamID, StreamCompleted)
}

func (s *memoryStreamStore) Cancel(ctx context.Context, sessionID, streamID string) (Stream, error) {
	return s.setState(ctx, sessionID, streamID, StreamCanceled)
}

func (s *memoryStreamStore) Expire(ctx context.Context, sessionID, streamID string) (Stream, error) {
	return s.setState(ctx, sessionID, streamID, StreamExpired)
}

func (s *memoryStreamStore) DeleteStream(ctx context.Context, sessionID, streamID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	if streams := s.streams[sessionID]; streams != nil {
		delete(streams, streamID)
		if len(streams) == 0 {
			delete(s.streams, sessionID)
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *memoryStreamStore) DeleteSession(ctx context.Context, sessionID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	delete(s.streams, sessionID)
	s.mu.Unlock()
	return nil
}

func (s *memoryStreamStore) setState(ctx context.Context, sessionID, streamID string, state StreamState) (Stream, error) {
	select {
	case <-ctx.Done():
		return Stream{}, ctx.Err()
	default:
	}
	if sessionID == "" || streamID == "" {
		return Stream{}, errStreamMissingFields
	}

	now := s.now()
	s.mu.Lock()
	s.pruneExpiredLocked(now)
	stream, ok := s.streams[sessionID][streamID]
	if !ok {
		s.mu.Unlock()
		return Stream{}, errStreamNotFound
	}
	if stream.State != StreamOpen && stream.State != state {
		s.mu.Unlock()
		return Stream{}, errStreamStateTransition
	}
	if stream.State == state {
		s.mu.Unlock()
		return stream, nil
	}
	stream.State = state
	stream.UpdatedAt = now
	stream.ExpiresAt = now.Add(s.ttl)
	s.streams[sessionID][streamID] = stream
	s.mu.Unlock()
	return stream, nil
}

func (s *memoryStreamStore) pruneExpiredLocked(now time.Time) {
	for sessionID, streams := range s.streams {
		for streamID, stream := range streams {
			if !now.Before(stream.ExpiresAt) {
				delete(streams, streamID)
			}
		}
		if len(streams) == 0 {
			delete(s.streams, sessionID)
		}
	}
}
