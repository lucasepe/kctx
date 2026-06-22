package queue

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestQueue(t *testing.T) {
	q := New(10, 10)
	q.Run()

	var count int64

	for i := 0; i < 10; i++ {
		job := NewJob("foo", func(ctx context.Context, v interface{}) {
			atomic.AddInt64(&count, 1)
		})
		if err := q.Push(context.Background(), job); err != nil {
			t.Fatalf("Push() error = %v", err)
		}
	}

	q.Terminate()

	if count != 10 {
		t.Error(count)
	}
}

func TestSyncQueue(t *testing.T) {
	q := New(1, 2)
	q.Run()
	defer q.Terminate()

	sjob := NewSyncJob("foo", func(ctx context.Context, v interface{}) (interface{}, error) {
		return fmt.Sprintf("%s_bar", v), nil
	})
	if err := q.Push(context.Background(), sjob); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	result := <-sjob.Wait()
	if err := sjob.Error(); err != nil {
		t.Error(err.Error())
	}

	if !reflect.DeepEqual(result, "foo_bar") {
		t.Error(result)
	}
}

func ExampleQueue() {
	q := New(10, 10)
	q.Run()

	var count int64

	for i := 0; i < 10; i++ {
		job := NewJob("foo", func(ctx context.Context, v interface{}) {
			atomic.AddInt64(&count, 1)
		})
		_ = q.Push(context.Background(), job)
	}

	q.Terminate()
	fmt.Println(count)
	// output: 10
}

func BenchmarkQueue(b *testing.B) {
	q := New(10, 100)
	q.Run()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			job := NewJob("", func(ctx context.Context, v interface{}) {
				_ = v
			})
			_ = q.Push(context.Background(), job)
		}
	})
	q.Terminate()
}

func TestQueuePushErrors(t *testing.T) {
	q := New(1, 1)
	if err := q.Push(context.Background(), NewJob("foo", func(context.Context, interface{}) {})); !errors.Is(err, ErrQueueNotRunning) {
		t.Fatalf("Push() error = %v, want %v", err, ErrQueueNotRunning)
	}

	q.Run()
	if err := q.Push(context.Background(), nil); !errors.Is(err, ErrNilJob) {
		t.Fatalf("Push(nil) error = %v, want %v", err, ErrNilJob)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.Push(ctx, NewJob("foo", func(context.Context, interface{}) {})); !errors.Is(err, context.Canceled) {
		t.Fatalf("Push(canceled) error = %v, want %v", err, context.Canceled)
	}
	q.Terminate()

	if err := q.Push(context.Background(), NewJob("foo", func(context.Context, interface{}) {})); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("Push(closed) error = %v, want %v", err, ErrQueueClosed)
	}
}

func TestQueuePushReturnsFull(t *testing.T) {
	q := New(1, 1)
	q.Run()
	defer q.Terminate()

	block := make(chan struct{})
	started := make(chan struct{})
	if err := q.Push(context.Background(), NewJob("foo", func(context.Context, interface{}) {
		close(started)
		<-block
	})); err != nil {
		t.Fatalf("Push(blocking job) error = %v", err)
	}
	<-started

	var err error
	for i := 0; i < 100; i++ {
		err = q.Push(context.Background(), NewJob("foo", func(context.Context, interface{}) {}))
		if errors.Is(err, ErrQueueFull) {
			break
		}
		if err != nil {
			t.Fatalf("Push() error = %v", err)
		}
	}
	if !errors.Is(err, ErrQueueFull) {
		close(block)
		t.Fatalf("Push() did not return %v", ErrQueueFull)
	}
	close(block)
}

func TestQueueJobPanicDoesNotStopWorker(t *testing.T) {
	q := New(2, 1)
	q.Run()
	defer q.Terminate()

	var count int64
	if err := q.Push(context.Background(), NewJob("panic", func(context.Context, interface{}) {
		panic("boom")
	})); err != nil {
		t.Fatalf("Push(panic) error = %v", err)
	}
	if err := q.Push(context.Background(), NewJob("ok", func(context.Context, interface{}) {
		atomic.AddInt64(&count, 1)
	})); err != nil {
		t.Fatalf("Push(ok) error = %v", err)
	}

	q.Terminate()
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}
