// Package workerpoolws - реализация обработчиков ws-сообщений из очереди
package workerpoolws

import "github.com/custom-app/sdk-go/service/wsservice/conn"

// Worker - обработчик простого сообщения
type Worker struct {
	queue   chan *conn.ReceivedMessage
	handler Handler
}

type Handler func(msg *conn.ReceivedMessage)

func NewWorker(queue chan *conn.ReceivedMessage, handler Handler) *Worker {
	return &Worker{
		queue:   queue,
		handler: handler,
	}
}

// Run - функция, запускающая вечный цикл, который будет слушать очередь до ее закрытия
func (w *Worker) Run() {
	for msg := range w.queue {
		w.handler(msg)
	}
}

// PrivateWorker - обработчик авторизованного сообщения
type PrivateWorker struct {
	queue   chan *conn.PrivateMessage
	handler PrivateHandler
}

type PrivateHandler func(msg *conn.PrivateMessage)

func NewPrivateWorker(queue chan *conn.PrivateMessage, handler PrivateHandler) *PrivateWorker {
	return &PrivateWorker{
		queue:   queue,
		handler: handler,
	}
}

// Run - функция, запускающая вечный цикл, который будет слушать очередь до ее закрытия
func (w *PrivateWorker) Run() {
	for msg := range w.queue {
		w.handler(msg)
	}
}

// PublicWorker - обработчик неавторизованного сообщения
type PublicWorker struct {
	queue   chan *conn.PublicMessage
	handler PublicHandler
}

type PublicHandler func(msg *conn.PublicMessage)

func NewPublicWorker(queue chan *conn.PublicMessage, handler PublicHandler) *PublicWorker {
	return &PublicWorker{
		queue:   queue,
		handler: handler,
	}
}

// Run - функция, запускающая вечный цикл, который будет слушать очередь до ее закрытия
func (w *PublicWorker) Run() {
	for msg := range w.queue {
		w.handler(msg)
	}
}

// ClientPrivateWorker - обработчик авторизованного сообщения из клиентского соединения
type ClientPrivateWorker struct {
	queue   chan *conn.ClientPrivateMessage
	handler ClientPrivateHandler
}

type ClientPrivateHandler func(msg *conn.ClientPrivateMessage)

func NewClientPrivateWorker(queue chan *conn.ClientPrivateMessage, handler ClientPrivateHandler) *ClientPrivateWorker {
	return &ClientPrivateWorker{
		queue:   queue,
		handler: handler,
	}
}

// Run - функция, запускающая вечный цикл, который будет слушать очередь до ее закрытия
func (w *ClientPrivateWorker) Run() {
	for msg := range w.queue {
		w.handler(msg)
	}
}

// ClientPublicWorker - обработчик неавторизованного сообщения из клиентского соединения
type ClientPublicWorker struct {
	queue   chan *conn.ClientPublicMessage
	handler ClientPublicHandler
}

type ClientPublicHandler func(msg *conn.ClientPublicMessage)

func NewClientPublicWorker(queue chan *conn.ClientPublicMessage, handler ClientPublicHandler) *ClientPublicWorker {
	return &ClientPublicWorker{
		queue:   queue,
		handler: handler,
	}
}

// Run - функция, запускающая вечный цикл, который будет слушать очередь до ее закрытия
func (w *ClientPublicWorker) Run() {
	for msg := range w.queue {
		w.handler(msg)
	}
}
