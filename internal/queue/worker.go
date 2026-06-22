package queue

import (
	"context"
	"log/slog"
	"sync"
)

func newWorker(ctx context.Context, pool chan chan Jober, wg *sync.WaitGroup) *worker {
	return &worker{
		ctx:     ctx,
		pool:    pool,
		wg:      wg,
		jobChan: make(chan Jober),
		quit:    make(chan struct{}),
	}
}

type worker struct {
	ctx     context.Context
	pool    chan chan Jober
	wg      *sync.WaitGroup
	jobChan chan Jober
	quit    chan struct{}
}

func (w *worker) Start() {
	w.pool <- w.jobChan
	go w.dispatcher()
}

func (w *worker) dispatcher() {
	for {
		select {
		case j := <-w.jobChan:
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Warn("queue job panicked", slog.Any("panic", r))
					}
				}()
				j.Job(w.ctx)
			}()
			w.pool <- w.jobChan
			w.wg.Done()
		case <-w.quit:
			<-w.pool
			close(w.jobChan)
			return
		}
	}
}

func (w *worker) Stop() {
	close(w.quit)
}
