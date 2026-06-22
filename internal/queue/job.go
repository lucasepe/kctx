package queue

import "context"

// Jober is a task that can be executed by a Queue worker.
type Jober interface {
	Job(ctx context.Context)
}

// SyncJober is a task that exposes its result after execution.
type SyncJober interface {
	Jober
	Wait() <-chan interface{}
	Error() error
}

type job struct {
	v        interface{}
	callback func(context.Context, interface{})
}

// NewJob creates a task backed by fn.
func NewJob(v interface{}, fn func(context.Context, interface{})) Jober {
	return &job{
		v:        v,
		callback: fn,
	}
}

func (j *job) Job(ctx context.Context) {
	j.callback(ctx, j.v)
}

type syncJob struct {
	err      error
	result   chan interface{}
	v        interface{}
	callback func(context.Context, interface{}) (interface{}, error)
}

// NewSyncJob creates a task that publishes one result or error.
func NewSyncJob(v interface{}, fn func(context.Context, interface{}) (interface{}, error)) SyncJober {
	return &syncJob{
		result:   make(chan interface{}, 1),
		v:        v,
		callback: fn,
	}
}

func (j *syncJob) Job(ctx context.Context) {
	result, err := j.callback(ctx, j.v)
	if err != nil {
		j.err = err
		close(j.result)
		return
	}

	j.result <- result

	close(j.result)
}

func (j *syncJob) Wait() <-chan interface{} {
	return j.result
}

func (j *syncJob) Error() error {
	return j.err
}
