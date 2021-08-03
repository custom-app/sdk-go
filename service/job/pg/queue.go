package pg

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v4"
	"time"
)

var (
	TimeoutErr = errors.New("timeout")
)

type Task struct {
	Ctx                   context.Context
	Options               pgx.TxOptions
	QueueTimeout, Timeout time.Duration
	Worker                DatabaseWorker
	returnCh              chan error
}

type Queue struct {
	queue chan *Task
}

func NewQueue(size int) *Queue {
	return &Queue{
		queue: make(chan *Task, size),
	}
}

func (q *Queue) AddJob(t *Task) {
	select {
	case q.queue <- t:
		return
	case <-t.Ctx.Done():
		t.returnCh <- TimeoutErr
	case <-time.After(t.QueueTimeout):
		t.returnCh <- TimeoutErr
	}
}

func (q *Queue) GetQueue() chan *Task {
	return q.queue
}

func (q *Queue) MakeJob(t *Task) error {
	t.returnCh = make(chan error)
	go q.AddJob(t)
	return <-t.returnCh
}

func (q *Queue) Close() {
	close(q.queue)
}

type Worker struct {
	queue chan *Task
}

func NewWorker(queue chan *Task) *Worker {
	return &Worker{
		queue: queue,
	}
}

func (w *Worker) Run() {
	for t := range w.queue {
		t.returnCh <- MakeJob(t.Ctx, t.Options, t.Worker, t.Timeout)
	}
}
