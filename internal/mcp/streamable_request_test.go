package mcp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMemoryStreamRequestStoreCreateAndGet(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	req, err := store.Create(context.Background(), "session-a", "stream-a", "request-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if req.SessionID != "session-a" {
		t.Fatalf("SessionID = %q, want session-a", req.SessionID)
	}
	if req.StreamID != "stream-a" {
		t.Fatalf("StreamID = %q, want stream-a", req.StreamID)
	}
	if req.RequestID != "request-1" {
		t.Fatalf("RequestID = %q, want request-1", req.RequestID)
	}
	if req.State != StreamRequestRunning {
		t.Fatalf("State = %q, want %q", req.State, StreamRequestRunning)
	}
	if !req.StartedAt.Equal(now) || !req.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps = %v/%v, want %v", req.StartedAt, req.UpdatedAt, now)
	}
	if !req.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("ExpiresAt = %v, want %v", req.ExpiresAt, now.Add(time.Minute))
	}

	got, ok := store.Get(context.Background(), "session-a", "stream-a")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got.RequestID != req.RequestID {
		t.Fatalf("Get() RequestID = %q, want %q", got.RequestID, req.RequestID)
	}
}

func TestMemoryStreamRequestStoreFindByRequest(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)
	req, err := store.Create(context.Background(), "session-a", "stream-a", "request-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := store.Create(context.Background(), "session-b", "stream-b", "request-1"); err != nil {
		t.Fatalf("Create() session-b error = %v", err)
	}

	got, ok := store.FindByRequest(context.Background(), "session-a", "request-1")
	if !ok {
		t.Fatal("FindByRequest() ok = false, want true")
	}
	if got.StreamID != req.StreamID {
		t.Fatalf("FindByRequest() StreamID = %q, want %q", got.StreamID, req.StreamID)
	}
}

func TestMemoryStreamRequestStoreRejectsDuplicateRequestInSession(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)
	if _, err := store.Create(context.Background(), "session-a", "stream-a", "request-1"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err := store.Create(context.Background(), "session-a", "stream-b", "request-1")
	if !errors.Is(err, errStreamRequestDuplicateRequest) {
		t.Fatalf("Create() duplicate error = %v, want %v", err, errStreamRequestDuplicateRequest)
	}
}

func TestMemoryStreamRequestStoreRequiresFields(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)

	if _, err := store.Create(context.Background(), "", "stream-a", "request-1"); !errors.Is(err, errStreamRequestMissingFields) {
		t.Fatalf("Create() missing session error = %v, want %v", err, errStreamRequestMissingFields)
	}
	if _, err := store.Create(context.Background(), "session-a", "", "request-1"); !errors.Is(err, errStreamRequestMissingFields) {
		t.Fatalf("Create() missing stream error = %v, want %v", err, errStreamRequestMissingFields)
	}
	if _, err := store.Create(context.Background(), "session-a", "stream-a", ""); !errors.Is(err, errStreamRequestMissingFields) {
		t.Fatalf("Create() missing request error = %v, want %v", err, errStreamRequestMissingFields)
	}
	if _, err := store.Complete(context.Background(), "", "stream-a"); !errors.Is(err, errStreamRequestMissingFields) {
		t.Fatalf("Complete() missing fields error = %v, want %v", err, errStreamRequestMissingFields)
	}
}

func TestMemoryStreamRequestStoreCompletesRequest(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	if _, err := store.Create(context.Background(), "session-a", "stream-a", "request-1"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	now = now.Add(10 * time.Second)
	completed, err := store.Complete(context.Background(), "session-a", "stream-a")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if completed.State != StreamRequestCompleted {
		t.Fatalf("State = %q, want %q", completed.State, StreamRequestCompleted)
	}
	if !completed.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %v, want %v", completed.UpdatedAt, now)
	}
	if !completed.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("ExpiresAt = %v, want renewed expiry", completed.ExpiresAt)
	}

	again, err := store.Complete(context.Background(), "session-a", "stream-a")
	if err != nil {
		t.Fatalf("Complete() idempotent error = %v", err)
	}
	if again.State != StreamRequestCompleted {
		t.Fatalf("idempotent State = %q, want %q", again.State, StreamRequestCompleted)
	}
}

func TestMemoryStreamRequestStoreRejectsTerminalStateChange(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)
	if _, err := store.Create(context.Background(), "session-a", "stream-a", "request-1"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := store.Complete(context.Background(), "session-a", "stream-a"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	_, err := store.Cancel(context.Background(), "session-a", "stream-a")
	if !errors.Is(err, errStreamRequestStateTransition) {
		t.Fatalf("Cancel() error = %v, want %v", err, errStreamRequestStateTransition)
	}
}

func TestMemoryStreamRequestStoreCancelAndExpire(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)
	if _, err := store.Create(context.Background(), "session-a", "stream-a", "request-1"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := store.Create(context.Background(), "session-a", "stream-b", "request-2"); err != nil {
		t.Fatalf("Create() second error = %v", err)
	}

	canceled, err := store.Cancel(context.Background(), "session-a", "stream-a")
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if canceled.State != StreamRequestCanceled {
		t.Fatalf("Cancel() State = %q, want %q", canceled.State, StreamRequestCanceled)
	}
	expired, err := store.Expire(context.Background(), "session-a", "stream-b")
	if err != nil {
		t.Fatalf("Expire() error = %v", err)
	}
	if expired.State != StreamRequestExpired {
		t.Fatalf("Expire() State = %q, want %q", expired.State, StreamRequestExpired)
	}
}

func TestMemoryStreamRequestStoreReturnsNotFoundForMissingRequest(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)

	_, err := store.Complete(context.Background(), "session-a", "stream-a")
	if !errors.Is(err, errStreamRequestNotFound) {
		t.Fatalf("Complete() error = %v, want %v", err, errStreamRequestNotFound)
	}
}

func TestMemoryStreamRequestStoreExpiresRequests(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	if _, err := store.Create(context.Background(), "session-a", "stream-a", "request-1"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	now = now.Add(time.Minute)

	if _, ok := store.Get(context.Background(), "session-a", "stream-a"); ok {
		t.Fatal("Get() ok = true after expiry, want false")
	}
	if _, ok := store.FindByRequest(context.Background(), "session-a", "request-1"); ok {
		t.Fatal("FindByRequest() ok = true after expiry, want false")
	}
}

func TestMemoryStreamRequestStoreDeleteStream(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)
	if _, err := store.Create(context.Background(), "session-a", "stream-a", "request-1"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := store.Create(context.Background(), "session-a", "stream-b", "request-2"); err != nil {
		t.Fatalf("Create() second error = %v", err)
	}

	if err := store.DeleteStream(context.Background(), "session-a", "stream-a"); err != nil {
		t.Fatalf("DeleteStream() error = %v", err)
	}
	if _, ok := store.Get(context.Background(), "session-a", "stream-a"); ok {
		t.Fatal("deleted request still exists")
	}
	if _, ok := store.Get(context.Background(), "session-a", "stream-b"); !ok {
		t.Fatal("kept request missing")
	}
}

func TestMemoryStreamRequestStoreDeleteSession(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)
	if _, err := store.Create(context.Background(), "session-a", "stream-a", "request-1"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.DeleteSession(context.Background(), "session-a"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if _, ok := store.Get(context.Background(), "session-a", "stream-a"); ok {
		t.Fatal("request exists after session delete")
	}
}

func TestMemoryStreamRequestStoreHonorsCanceledContext(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Create(ctx, "session-a", "stream-a", "request-1"); err == nil {
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

func TestMemoryStreamRequestStoreConcurrentAccess(t *testing.T) {
	store := newMemoryStreamRequestStore(time.Minute)

	const workers = 16
	var wg sync.WaitGroup
	streamIDs := make(chan string, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			streamID := string(rune('a' + i))
			requestID := string(rune('A' + i))
			if _, err := store.Create(context.Background(), "session-a", streamID, requestID); err != nil {
				t.Errorf("Create() error = %v", err)
				return
			}
			if _, ok := store.Get(context.Background(), "session-a", streamID); !ok {
				t.Errorf("Get(%q) ok = false, want true", streamID)
				return
			}
			if _, err := store.Complete(context.Background(), "session-a", streamID); err != nil {
				t.Errorf("Complete(%q) error = %v", streamID, err)
				return
			}
			streamIDs <- streamID
		}(i)
	}

	wg.Wait()
	close(streamIDs)

	for streamID := range streamIDs {
		req, ok := store.Get(context.Background(), "session-a", streamID)
		if !ok {
			t.Fatalf("Get(%q) ok = false, want true", streamID)
		}
		if req.State != StreamRequestCompleted {
			t.Fatalf("Get(%q).State = %q, want %q", streamID, req.State, StreamRequestCompleted)
		}
	}
}
