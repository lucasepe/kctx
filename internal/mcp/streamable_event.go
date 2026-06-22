package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	errStreamEventMissingFields = errors.New("stream event requires id, session id, and stream id")
	errStreamEventNotFound      = errors.New("stream event not found")
	errStreamEventDuplicateID   = errors.New("stream event id already exists")
)

// StreamEvent represents one retained MCP Streamable HTTP event.
//
// The event ID is an opaque cursor for clients. Stores must only replay events
// from the same session and stream that produced the cursor.
type StreamEvent struct {
	ID        string
	SessionID string
	StreamID  string
	RequestID string
	Data      json.RawMessage
	CreatedAt time.Time
}

// StreamEventStore stores recent Streamable HTTP events for resumability.
//
// Implementations may keep events in memory, Redis, or another shared store.
// The HTTP transport depends only on this interface so replay storage can
// become persistent later without changing request handling.
type StreamEventStore interface {
	Append(ctx context.Context, event StreamEvent) error
	After(ctx context.Context, sessionID, streamID, lastEventID string) ([]StreamEvent, error)
	DeleteStream(ctx context.Context, sessionID, streamID string) error
	DeleteSession(ctx context.Context, sessionID string) error
}

// StreamEventIDGenerator creates ordered event IDs for one logical stream.
type StreamEventIDGenerator interface {
	Next(streamID string) string
}

type memoryStreamEventStore struct {
	mu     sync.RWMutex
	ttl    time.Duration
	now    func() time.Time
	events map[string]map[string][]StreamEvent
}

func newMemoryStreamEventStore(ttl time.Duration) *memoryStreamEventStore {
	if ttl <= 0 {
		ttl = streamableSessionTTL
	}
	return &memoryStreamEventStore{
		ttl:    ttl,
		now:    time.Now,
		events: map[string]map[string][]StreamEvent{},
	}
}

func (s *memoryStreamEventStore) Append(ctx context.Context, event StreamEvent) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if event.ID == "" || event.SessionID == "" || event.StreamID == "" {
		return errStreamEventMissingFields
	}

	now := s.now()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	event.Data = cloneRawMessage(event.Data)

	s.mu.Lock()
	s.pruneExpiredLocked(now)
	streams := s.events[event.SessionID]
	if streams == nil {
		streams = map[string][]StreamEvent{}
		s.events[event.SessionID] = streams
	}
	for _, existing := range streams[event.StreamID] {
		if existing.ID == event.ID {
			s.mu.Unlock()
			return errStreamEventDuplicateID
		}
	}
	streams[event.StreamID] = append(streams[event.StreamID], event)
	s.mu.Unlock()
	return nil
}

func (s *memoryStreamEventStore) After(ctx context.Context, sessionID, streamID, lastEventID string) ([]StreamEvent, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if sessionID == "" || streamID == "" {
		return nil, errStreamEventMissingFields
	}

	now := s.now()
	s.mu.Lock()
	s.pruneExpiredLocked(now)
	events := s.events[sessionID][streamID]
	if len(events) == 0 {
		s.mu.Unlock()
		if lastEventID == "" {
			return nil, nil
		}
		return nil, errStreamEventNotFound
	}
	if lastEventID == "" {
		replay := cloneEvents(events)
		s.mu.Unlock()
		return replay, nil
	}

	for i, event := range events {
		if event.ID == lastEventID {
			replay := cloneEvents(events[i+1:])
			s.mu.Unlock()
			return replay, nil
		}
	}
	s.mu.Unlock()
	return nil, errStreamEventNotFound
}

func (s *memoryStreamEventStore) DeleteStream(ctx context.Context, sessionID, streamID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	if streams := s.events[sessionID]; streams != nil {
		delete(streams, streamID)
		if len(streams) == 0 {
			delete(s.events, sessionID)
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *memoryStreamEventStore) DeleteSession(ctx context.Context, sessionID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	delete(s.events, sessionID)
	s.mu.Unlock()
	return nil
}

func (s *memoryStreamEventStore) pruneExpiredLocked(now time.Time) {
	for sessionID, streams := range s.events {
		for streamID, events := range streams {
			kept := events[:0]
			for _, event := range events {
				if now.Sub(event.CreatedAt) < s.ttl {
					kept = append(kept, event)
				}
			}
			if len(kept) == 0 {
				delete(streams, streamID)
				continue
			}
			streams[streamID] = kept
		}
		if len(streams) == 0 {
			delete(s.events, sessionID)
		}
	}
}

type memoryStreamEventIDGenerator struct {
	mu        sync.Mutex
	sequences map[string]uint64
}

func newMemoryStreamEventIDGenerator() *memoryStreamEventIDGenerator {
	return &memoryStreamEventIDGenerator{sequences: map[string]uint64{}}
}

func (g *memoryStreamEventIDGenerator) Next(streamID string) string {
	g.mu.Lock()
	g.sequences[streamID]++
	sequence := g.sequences[streamID]
	g.mu.Unlock()
	return formatStreamEventID(streamID, sequence)
}

func formatStreamEventID(streamID string, sequence uint64) string {
	return fmt.Sprintf("%s:%020d", streamID, sequence)
}

func cloneEvents(events []StreamEvent) []StreamEvent {
	if len(events) == 0 {
		return nil
	}
	cloned := make([]StreamEvent, len(events))
	for i, event := range events {
		cloned[i] = event
		cloned[i].Data = cloneRawMessage(event.Data)
	}
	return cloned
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	cloned := make(json.RawMessage, len(raw))
	copy(cloned, raw)
	return cloned
}
