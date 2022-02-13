package mongo

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/mongo"
	"time"
)

var (
	TimeoutErr = errors.New("timeout")
)

// Task - задача по работе с БД
type Task struct {
	Ctx                   context.Context // Контекст
	Session               mongo.Session   // Сессия
	QueueTimeout, Timeout time.Duration   // Таймаут попадания в очередь и таймаут транзакции
	Worker                DatabaseWorker  // Действие с транзакцией
	returnCh              chan error
}

// Queue - очередь запросов в БД
type Queue struct {
	queue chan *Task
}

func NewQueue(size int) *Queue {
	return &Queue{
		queue: make(chan *Task, size),
	}
}

func (q *Queue) addJob(t *Task) {
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
	go q.addJob(t)
	return <-t.returnCh
}

func (q *Queue) Close() {
	close(q.queue)
}

// Worker - обработчик запроса в БД. Берет запросы из очереди, и запускает выполнение работы
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
		t.returnCh <- MakeJob(t.Ctx, t.Worker, t.Timeout)
	}
}
