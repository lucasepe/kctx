package mcp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMemoryStreamStoreCreateAndGet(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	stream, err := store.Create(context.Background(), "session-a", "request-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if stream.ID == "" {
		t.Fatal("stream ID is empty")
	}
	if stream.SessionID != "session-a" {
		t.Fatalf("SessionID = %q, want session-a", stream.SessionID)
	}
	if stream.RequestID != "request-1" {
		t.Fatalf("RequestID = %q, want request-1", stream.RequestID)
	}
	if stream.State != StreamOpen {
		t.Fatalf("State = %q, want %q", stream.State, StreamOpen)
	}
	if !stream.CreatedAt.Equal(now) || !stream.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps = %v/%v, want %v", stream.CreatedAt, stream.UpdatedAt, now)
	}
	if !stream.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("ExpiresAt = %v, want %v", stream.ExpiresAt, now.Add(time.Minute))
	}

	got, ok := store.Get(context.Background(), "session-a", stream.ID)
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got.ID != stream.ID {
		t.Fatalf("Get() ID = %q, want %q", got.ID, stream.ID)
	}
}

func TestMemoryStreamStoreFindByRequest(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)
	stream, err := store.Create(context.Background(), "session-a", "request-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := store.Create(context.Background(), "session-b", "request-1"); err != nil {
		t.Fatalf("Create() session-b error = %v", err)
	}

	got, ok := store.FindByRequest(context.Background(), "session-a", "request-1")
	if !ok {
		t.Fatal("FindByRequest() ok = false, want true")
	}
	if got.ID != stream.ID {
		t.Fatalf("FindByRequest() ID = %q, want %q", got.ID, stream.ID)
	}
}

func TestMemoryStreamStoreRejectsDuplicateRequestInSession(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)
	if _, err := store.Create(context.Background(), "session-a", "request-1"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err := store.Create(context.Background(), "session-a", "request-1")
	if !errors.Is(err, errStreamRequestDuplicate) {
		t.Fatalf("Create() duplicate error = %v, want %v", err, errStreamRequestDuplicate)
	}
}

func TestMemoryStreamStoreRequiresSessionAndRequest(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)

	if _, err := store.Create(context.Background(), "", "request-1"); !errors.Is(err, errStreamSessionRequired) {
		t.Fatalf("Create() missing session error = %v, want %v", err, errStreamSessionRequired)
	}
	if _, err := store.Create(context.Background(), "session-a", ""); !errors.Is(err, errStreamRequestIDRequired) {
		t.Fatalf("Create() missing request error = %v, want %v", err, errStreamRequestIDRequired)
	}
	if _, err := store.Complete(context.Background(), "", "stream-a"); !errors.Is(err, errStreamMissingFields) {
		t.Fatalf("Complete() missing fields error = %v, want %v", err, errStreamMissingFields)
	}
}

func TestMemoryStreamStoreCompletesStream(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	stream, err := store.Create(context.Background(), "session-a", "request-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	now = now.Add(10 * time.Second)
	completed, err := store.Complete(context.Background(), "session-a", stream.ID)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if completed.State != StreamCompleted {
		t.Fatalf("State = %q, want %q", completed.State, StreamCompleted)
	}
	if !completed.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %v, want %v", completed.UpdatedAt, now)
	}
	if !completed.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("ExpiresAt = %v, want renewed expiry", completed.ExpiresAt)
	}

	again, err := store.Complete(context.Background(), "session-a", stream.ID)
	if err != nil {
		t.Fatalf("Complete() idempotent error = %v", err)
	}
	if again.State != StreamCompleted {
		t.Fatalf("idempotent State = %q, want %q", again.State, StreamCompleted)
	}
}

func TestMemoryStreamStoreRejectsTerminalStateChange(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)
	stream, err := store.Create(context.Background(), "session-a", "request-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := store.Complete(context.Background(), "session-a", stream.ID); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	_, err = store.Cancel(context.Background(), "session-a", stream.ID)
	if !errors.Is(err, errStreamStateTransition) {
		t.Fatalf("Cancel() error = %v, want %v", err, errStreamStateTransition)
	}
}

func TestMemoryStreamStoreCancelAndExpire(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)
	canceled, err := store.Create(context.Background(), "session-a", "request-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	expired, err := store.Create(context.Background(), "session-a", "request-2")
	if err != nil {
		t.Fatalf("Create() second error = %v", err)
	}

	canceled, err = store.Cancel(context.Background(), "session-a", canceled.ID)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if canceled.State != StreamCanceled {
		t.Fatalf("Cancel() State = %q, want %q", canceled.State, StreamCanceled)
	}
	expired, err = store.Expire(context.Background(), "session-a", expired.ID)
	if err != nil {
		t.Fatalf("Expire() error = %v", err)
	}
	if expired.State != StreamExpired {
		t.Fatalf("Expire() State = %q, want %q", expired.State, StreamExpired)
	}
}

func TestMemoryStreamStoreReturnsNotFoundForMissingStream(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)

	_, err := store.Complete(context.Background(), "session-a", "stream-a")
	if !errors.Is(err, errStreamNotFound) {
		t.Fatalf("Complete() error = %v, want %v", err, errStreamNotFound)
	}
}

func TestMemoryStreamStoreExpiresStreams(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	stream, err := store.Create(context.Background(), "session-a", "request-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	now = now.Add(time.Minute)

	if _, ok := store.Get(context.Background(), "session-a", stream.ID); ok {
		t.Fatal("Get() ok = true after expiry, want false")
	}
	if _, ok := store.FindByRequest(context.Background(), "session-a", "request-1"); ok {
		t.Fatal("FindByRequest() ok = true after expiry, want false")
	}
}

func TestMemoryStreamStoreDeleteStream(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)
	deleted, err := store.Create(context.Background(), "session-a", "request-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	kept, err := store.Create(context.Background(), "session-a", "request-2")
	if err != nil {
		t.Fatalf("Create() second error = %v", err)
	}

	if err := store.DeleteStream(context.Background(), "session-a", deleted.ID); err != nil {
		t.Fatalf("DeleteStream() error = %v", err)
	}
	if _, ok := store.Get(context.Background(), "session-a", deleted.ID); ok {
		t.Fatal("deleted stream still exists")
	}
	if _, ok := store.Get(context.Background(), "session-a", kept.ID); !ok {
		t.Fatal("kept stream missing")
	}
}

func TestMemoryStreamStoreDeleteSession(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)
	stream, err := store.Create(context.Background(), "session-a", "request-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.DeleteSession(context.Background(), "session-a"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if _, ok := store.Get(context.Background(), "session-a", stream.ID); ok {
		t.Fatal("stream exists after session delete")
	}
}

func TestMemoryStreamStoreHonorsCanceledContext(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Create(ctx, "session-a", "request-1"); err == nil {
		t.Fatal("Create() error = nil, want context cancellation error")
	}
	if _, ok := store.Get(ctx, "session-a", "stream-a"); ok {
		t.Fatal("Get() ok = true with canceled context, want false")
	}
	if _, ok := store.FindByRequest(ctx, "session-a", "request-1"); ok {
		t.Fatal("FindByRequest() ok = true with canceled context, want false")
	}
	if _, err := store.Complete(ctx, "session-a", "stream-a"); err == nil {
		t.Fatal("Complete() error = nil, want context cancellation error")
	}
	if err := store.DeleteStream(ctx, "session-a", "stream-a"); err == nil {
		t.Fatal("DeleteStream() error = nil, want context cancellation error")
	}
	if err := store.DeleteSession(ctx, "session-a"); err == nil {
		t.Fatal("DeleteSession() error = nil, want context cancellation error")
	}
}

func TestMemoryStreamStoreConcurrentAccess(t *testing.T) {
	store := newMemoryStreamStore(time.Minute)

	const workers = 16
	var wg sync.WaitGroup
	ids := make(chan string, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			stream, err := store.Create(context.Background(), "session-a", string(rune('a'+i)))
			if err != nil {
				t.Errorf("Create() error = %v", err)
				return
			}
			if _, ok := store.Get(context.Background(), "session-a", stream.ID); !ok {
				t.Errorf("Get(%q) ok = false, want true", stream.ID)
				return
			}
			if _, err := store.Complete(context.Background(), "session-a", stream.ID); err != nil {
				t.Errorf("Complete(%q) error = %v", stream.ID, err)
				return
			}
			ids <- stream.ID
		}(i)
	}

	wg.Wait()
	close(ids)

	for id := range ids {
		stream, ok := store.Get(context.Background(), "session-a", id)
		if !ok {
			t.Fatalf("Get(%q) ok = false, want true", id)
		}
		if stream.State != StreamCompleted {
			t.Fatalf("Get(%q).State = %q, want %q", id, stream.State, StreamCompleted)
		}
	}
}
