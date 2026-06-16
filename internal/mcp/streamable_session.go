package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// StreamableSession represents one MCP Streamable HTTP session.
type StreamableSession struct {
	ID        string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// StreamableSessionStore stores MCP Streamable HTTP sessions.
//
// Implementations may keep sessions in memory, Redis, or another shared store.
// The transport only depends on this small interface so session storage can
// change without changing request handling.
type StreamableSessionStore interface {
	Create(ctx context.Context) (StreamableSession, error)
	Get(id string) (StreamableSession, bool)
	Delete(id string) bool
}

type memoryStreamableSessionStore struct {
	mu       sync.RWMutex
	ttl      time.Duration
	now      func() time.Time
	sessions map[string]StreamableSession
}

func newMemoryStreamableSessionStore(ttl time.Duration) *memoryStreamableSessionStore {
	if ttl <= 0 {
		ttl = streamableSessionTTL
	}
	return &memoryStreamableSessionStore{
		ttl:      ttl,
		now:      time.Now,
		sessions: map[string]StreamableSession{},
	}
}

func (s *memoryStreamableSessionStore) Create(ctx context.Context) (StreamableSession, error) {
	select {
	case <-ctx.Done():
		return StreamableSession{}, ctx.Err()
	default:
	}

	id, err := newSessionID()
	if err != nil {
		return StreamableSession{}, err
	}
	now := s.now()
	session := StreamableSession{
		ID:        id,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}

	s.mu.Lock()
	s.sessions[id] = session
	s.pruneExpiredLocked(now)
	s.mu.Unlock()
	return session, nil
}

func (s *memoryStreamableSessionStore) Get(id string) (StreamableSession, bool) {
	if id == "" {
		return StreamableSession{}, false
	}

	now := s.now()
	s.mu.RLock()
	session, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return StreamableSession{}, false
	}
	if !now.Before(session.ExpiresAt) {
		s.Delete(id)
		return StreamableSession{}, false
	}
	return session, true
}

func (s *memoryStreamableSessionStore) Delete(id string) bool {
	if id == "" {
		return false
	}

	s.mu.Lock()
	_, ok := s.sessions[id]
	delete(s.sessions, id)
	s.mu.Unlock()
	return ok
}

func (s *memoryStreamableSessionStore) pruneExpiredLocked(now time.Time) {
	for id, session := range s.sessions {
		if !now.Before(session.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}

// newSessionID returns a random hexadecimal session identifier.
func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
