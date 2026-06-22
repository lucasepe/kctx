package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	// ErrQueueClosed indicates that the queue has already been terminated.
	ErrQueueClosed = errors.New("queue is closed")
	// ErrQueueFull indicates that the bounded queue cannot accept more work.
	ErrQueueFull = errors.New("queue is full")
	// ErrQueueNotRunning indicates that Run has not been called.
	ErrQueueNotRunning = errors.New("queue is not running")
	// ErrNilJob indicates that Push received a nil job.
	ErrNilJob = errors.New("queue job is nil")
)

// New creates a bounded queue with maxCapacity buffered jobs and maxThread workers.
func New(maxCapacity, maxThread int) *Queue {
	return &Queue{
		jobQueue:   make(chan Jober, maxCapacity),
		maxWorkers: maxThread,
		workerPool: make(chan chan Jober, maxThread),
		workers:    make([]*worker, maxThread),
		wg:         new(sync.WaitGroup),
		done:       make(chan struct{}),
	}
}

// Queue runs submitted jobs with a fixed-size worker pool.
type Queue struct {
	maxWorkers int
	jobQueue   chan Jober
	workerPool chan chan Jober
	workers    []*worker
	running    uint32
	closed     uint32
	enqueueMu  sync.RWMutex
	wg         *sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
}

// Run starts the queue workers. Calling Run more than once is a no-op.
func (q *Queue) Run() {
	if atomic.LoadUint32(&q.closed) == 1 || atomic.LoadUint32(&q.running) == 1 {
		return
	}

	q.ctx, q.cancel = context.WithCancel(context.Background())
	atomic.StoreUint32(&q.running, 1)
	for i := 0; i < q.maxWorkers; i++ {
		q.workers[i] = newWorker(q.ctx, q.workerPool, q.wg)
		q.workers[i].Start()
	}

	go q.dispatcher()
}

func (q *Queue) dispatcher() {
	defer close(q.done)
	for job := range q.jobQueue {
		worker := <-q.workerPool
		worker <- job
	}
}

// Terminate stops accepting jobs and waits for accepted jobs to finish.
func (q *Queue) Terminate() {
	if atomic.LoadUint32(&q.running) != 1 {
		return
	}

	atomic.StoreUint32(&q.running, 0)
	atomic.StoreUint32(&q.closed, 1)
	q.enqueueMu.Lock()
	close(q.jobQueue)
	q.enqueueMu.Unlock()
	q.wg.Wait()
	if q.cancel != nil {
		q.cancel()
	}
	<-q.done

	for i := 0; i < q.maxWorkers; i++ {
		q.workers[i].Stop()
	}
	close(q.workerPool)
}

// Push enqueues job without blocking when the queue is full.
func (q *Queue) Push(ctx context.Context, job Jober) error {
	if job == nil {
		return ErrNilJob
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if atomic.LoadUint32(&q.closed) == 1 {
		return ErrQueueClosed
	}
	if atomic.LoadUint32(&q.running) != 1 {
		return ErrQueueNotRunning
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	q.enqueueMu.RLock()
	defer q.enqueueMu.RUnlock()
	if atomic.LoadUint32(&q.closed) == 1 {
		return ErrQueueClosed
	}
	q.wg.Add(1)
	select {
	case <-ctx.Done():
		q.wg.Done()
		return ctx.Err()
	case q.jobQueue <- job:
		return nil
	default:
		q.wg.Done()
		return ErrQueueFull
	}
}

func (q *Queue) GetJobCount() int {
	return len(q.jobQueue)
}
