package mcp

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryStreamableSessionStoreCreateAndGet(t *testing.T) {
	store := newMemoryStreamableSessionStore(time.Minute)
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	session, err := store.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if session.ID == "" {
		t.Fatal("session ID is empty")
	}
	if !session.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v, want %v", session.CreatedAt, now)
	}
	if !session.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("ExpiresAt = %v, want %v", session.ExpiresAt, now.Add(time.Minute))
	}

	got, ok := store.Get(session.ID)
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got.ID != session.ID {
		t.Fatalf("Get() ID = %q, want %q", got.ID, session.ID)
	}
}

func TestMemoryStreamableSessionStoreDelete(t *testing.T) {
	store := newMemoryStreamableSessionStore(time.Minute)
	session, err := store.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if ok := store.Delete(session.ID); !ok {
		t.Fatal("Delete() ok = false, want true")
	}
	if _, ok := store.Get(session.ID); ok {
		t.Fatal("Get() ok = true after delete, want false")
	}
	if ok := store.Delete(session.ID); ok {
		t.Fatal("Delete() ok = true for missing session, want false")
	}
}

func TestMemoryStreamableSessionStoreExpiresSessions(t *testing.T) {
	store := newMemoryStreamableSessionStore(time.Minute)
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	session, err := store.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, ok := store.Get(session.ID); !ok {
		t.Fatal("session was not found before expiry")
	}

	now = now.Add(time.Minute)
	if _, ok := store.Get(session.ID); ok {
		t.Fatal("session was found after expiry")
	}
}

func TestMemoryStreamableSessionStorePrunesExpiredOnCreate(t *testing.T) {
	store := newMemoryStreamableSessionStore(time.Minute)
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	expired, err := store.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	now = now.Add(time.Minute)
	active, err := store.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, ok := store.Get(expired.ID); ok {
		t.Fatal("expired session was not pruned")
	}
	if _, ok := store.Get(active.ID); !ok {
		t.Fatal("active session was not found")
	}
}

func TestMemoryStreamableSessionStoreCreateHonorsCanceledContext(t *testing.T) {
	store := newMemoryStreamableSessionStore(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Create(ctx); err == nil {
		t.Fatal("Create() error = nil, want context cancellation error")
	}
}

func TestMemoryStreamableSessionStoreConcurrentAccess(t *testing.T) {
	store := newMemoryStreamableSessionStore(time.Minute)

	const workers = 16
	var wg sync.WaitGroup
	ids := make(chan string, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session, err := store.Create(context.Background())
			if err != nil {
				t.Errorf("Create() error = %v", err)
				return
			}
			if _, ok := store.Get(session.ID); !ok {
				t.Errorf("Get(%q) ok = false, want true", session.ID)
				return
			}
			ids <- session.ID
		}()
	}

	wg.Wait()
	close(ids)

	for id := range ids {
		if ok := store.Delete(id); !ok {
			t.Fatalf("Delete(%q) ok = false, want true", id)
		}
	}
}
