package mcp

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMemoryStreamEventStoreAppendAndReplay(t *testing.T) {
	store := newMemoryStreamEventStore(time.Minute)
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	events := []StreamEvent{
		{ID: "stream-a:00000000000000000001", SessionID: "session-a", StreamID: "stream-a", RequestID: "1", Data: []byte(`{"step":1}`)},
		{ID: "stream-a:00000000000000000002", SessionID: "session-a", StreamID: "stream-a", RequestID: "1", Data: []byte(`{"step":2}`)},
		{ID: "stream-a:00000000000000000003", SessionID: "session-a", StreamID: "stream-a", RequestID: "1", Data: []byte(`{"step":3}`)},
	}
	for _, event := range events {
		if err := store.Append(context.Background(), event); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	got, err := store.After(context.Background(), "session-a", "stream-a", "")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if len(got) != len(events) {
		t.Fatalf("After() len = %d, want %d", len(got), len(events))
	}
	for i := range got {
		if got[i].ID != events[i].ID {
			t.Fatalf("After()[%d].ID = %q, want %q", i, got[i].ID, events[i].ID)
		}
		if !got[i].CreatedAt.Equal(now) {
			t.Fatalf("After()[%d].CreatedAt = %v, want %v", i, got[i].CreatedAt, now)
		}
	}
}

func TestMemoryStreamEventStoreAfterCursor(t *testing.T) {
	store := newMemoryStreamEventStore(time.Minute)
	appendStreamEvent(t, store, "session-a", "stream-a", "event-1")
	appendStreamEvent(t, store, "session-a", "stream-a", "event-2")
	appendStreamEvent(t, store, "session-a", "stream-a", "event-3")

	got, err := store.After(context.Background(), "session-a", "stream-a", "event-1")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("After() len = %d, want 2", len(got))
	}
	if got[0].ID != "event-2" || got[1].ID != "event-3" {
		t.Fatalf("After() IDs = %q, %q; want event-2, event-3", got[0].ID, got[1].ID)
	}
}

func TestMemoryStreamEventStoreRejectsUnknownCursor(t *testing.T) {
	store := newMemoryStreamEventStore(time.Minute)
	appendStreamEvent(t, store, "session-a", "stream-a", "event-1")
	appendStreamEvent(t, store, "session-a", "stream-b", "event-2")

	_, err := store.After(context.Background(), "session-a", "stream-a", "event-2")
	if !errors.Is(err, errStreamEventNotFound) {
		t.Fatalf("After() error = %v, want %v", err, errStreamEventNotFound)
	}
}

func TestMemoryStreamEventStoreRejectsDuplicateEventID(t *testing.T) {
	store := newMemoryStreamEventStore(time.Minute)
	appendStreamEvent(t, store, "session-a", "stream-a", "event-1")

	err := store.Append(context.Background(), StreamEvent{
		ID:        "event-1",
		SessionID: "session-a",
		StreamID:  "stream-a",
		Data:      []byte(`{}`),
	})
	if !errors.Is(err, errStreamEventDuplicateID) {
		t.Fatalf("Append() error = %v, want %v", err, errStreamEventDuplicateID)
	}
}

func TestMemoryStreamEventStoreExpiresEvents(t *testing.T) {
	store := newMemoryStreamEventStore(time.Minute)
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	appendStreamEvent(t, store, "session-a", "stream-a", "event-1")
	now = now.Add(time.Minute)

	got, err := store.After(context.Background(), "session-a", "stream-a", "")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("After() len = %d, want 0 after expiry", len(got))
	}
}

func TestMemoryStreamEventStoreDeleteStream(t *testing.T) {
	store := newMemoryStreamEventStore(time.Minute)
	appendStreamEvent(t, store, "session-a", "stream-a", "event-1")
	appendStreamEvent(t, store, "session-a", "stream-b", "event-2")

	if err := store.DeleteStream(context.Background(), "session-a", "stream-a"); err != nil {
		t.Fatalf("DeleteStream() error = %v", err)
	}
	got, err := store.After(context.Background(), "session-a", "stream-a", "")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("After() len = %d, want 0 for deleted stream", len(got))
	}
	got, err = store.After(context.Background(), "session-a", "stream-b", "")
	if err != nil {
		t.Fatalf("After() stream-b error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("After() stream-b len = %d, want 1", len(got))
	}
}

func TestMemoryStreamEventStoreDeleteSession(t *testing.T) {
	store := newMemoryStreamEventStore(time.Minute)
	appendStreamEvent(t, store, "session-a", "stream-a", "event-1")
	appendStreamEvent(t, store, "session-a", "stream-b", "event-2")

	if err := store.DeleteSession(context.Background(), "session-a"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	got, err := store.After(context.Background(), "session-a", "stream-a", "")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("After() len = %d, want 0 after session delete", len(got))
	}
}

func TestMemoryStreamEventStoreHonorsCanceledContext(t *testing.T) {
	store := newMemoryStreamEventStore(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.Append(ctx, StreamEvent{ID: "event-1", SessionID: "session-a", StreamID: "stream-a"}); err == nil {
		t.Fatal("Append() error = nil, want context cancellation error")
	}
	if _, err := store.After(ctx, "session-a", "stream-a", ""); err == nil {
		t.Fatal("After() error = nil, want context cancellation error")
	}
	if err := store.DeleteStream(ctx, "session-a", "stream-a"); err == nil {
		t.Fatal("DeleteStream() error = nil, want context cancellation error")
	}
	if err := store.DeleteSession(ctx, "session-a"); err == nil {
		t.Fatal("DeleteSession() error = nil, want context cancellation error")
	}
}

func TestMemoryStreamEventStoreRejectsMissingRequiredFields(t *testing.T) {
	store := newMemoryStreamEventStore(time.Minute)

	err := store.Append(context.Background(), StreamEvent{SessionID: "session-a", StreamID: "stream-a"})
	if !errors.Is(err, errStreamEventMissingFields) {
		t.Fatalf("Append() error = %v, want %v", err, errStreamEventMissingFields)
	}
	_, err = store.After(context.Background(), "", "stream-a", "")
	if !errors.Is(err, errStreamEventMissingFields) {
		t.Fatalf("After() error = %v, want %v", err, errStreamEventMissingFields)
	}
}

func TestMemoryStreamEventStoreCopiesData(t *testing.T) {
	store := newMemoryStreamEventStore(time.Minute)
	data := []byte(`{"ok":true}`)
	if err := store.Append(context.Background(), StreamEvent{
		ID:        "event-1",
		SessionID: "session-a",
		StreamID:  "stream-a",
		Data:      data,
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	data[6] = 'f'

	got, err := store.After(context.Background(), "session-a", "stream-a", "")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if !bytes.Equal(got[0].Data, []byte(`{"ok":true}`)) {
		t.Fatalf("stored Data = %s, want original value", got[0].Data)
	}

	got[0].Data[6] = 'f'
	got, err = store.After(context.Background(), "session-a", "stream-a", "")
	if err != nil {
		t.Fatalf("After() second error = %v", err)
	}
	if !bytes.Equal(got[0].Data, []byte(`{"ok":true}`)) {
		t.Fatalf("returned Data mutated store: %s", got[0].Data)
	}
}

func TestMemoryStreamEventStoreConcurrentAccess(t *testing.T) {
	store := newMemoryStreamEventStore(time.Minute)
	ids := newMemoryStreamEventIDGenerator()

	const workers = 16
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := ids.Next("stream-a")
			if err := store.Append(context.Background(), StreamEvent{
				ID:        id,
				SessionID: "session-a",
				StreamID:  "stream-a",
				Data:      []byte(`{}`),
			}); err != nil {
				t.Errorf("Append() error = %v", err)
			}
			if _, err := store.After(context.Background(), "session-a", "stream-a", ""); err != nil {
				t.Errorf("After() error = %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := store.After(context.Background(), "session-a", "stream-a", "")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if len(got) != workers {
		t.Fatalf("After() len = %d, want %d", len(got), workers)
	}
}

func TestMemoryStreamEventIDGenerator(t *testing.T) {
	generator := newMemoryStreamEventIDGenerator()

	if got := generator.Next("stream-a"); got != "stream-a:00000000000000000001" {
		t.Fatalf("Next() = %q, want first stream-a event", got)
	}
	if got := generator.Next("stream-a"); got != "stream-a:00000000000000000002" {
		t.Fatalf("Next() = %q, want second stream-a event", got)
	}
	if got := generator.Next("stream-b"); got != "stream-b:00000000000000000001" {
		t.Fatalf("Next() = %q, want first stream-b event", got)
	}
}

func appendStreamEvent(t *testing.T, store StreamEventStore, sessionID, streamID, eventID string) {
	t.Helper()
	if err := store.Append(context.Background(), StreamEvent{
		ID:        eventID,
		SessionID: sessionID,
		StreamID:  streamID,
		Data:      []byte(`{}`),
	}); err != nil {
		t.Fatalf("Append(%q) error = %v", eventID, err)
	}
}
