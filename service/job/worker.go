package job

import (
	"github.com/loyal-inform/sdk-go/service/ws/conn"
)

type HttpWorker struct {
	queue chan *HttpJob
}

func NewHttpWorker(queue chan *HttpJob) *HttpWorker {
	return &HttpWorker{
		queue: queue,
	}
}

func (w *HttpWorker) Run() {
	for j := range w.queue {
		j.Handler.ServeHTTP(j.W, j.R)
		j.resCh <- true
	}
}

type WsWorker struct {
	queue   chan *conn.ReceivedMessage
	handler WsHandler
}

type WsHandler func(msg *conn.ReceivedMessage)

func NewWsWorker(queue chan *conn.ReceivedMessage, handler WsHandler) *WsWorker {
	return &WsWorker{
		queue:   queue,
		handler: handler,
	}
}

func (w *WsWorker) Run() {
	for msg := range w.queue {
		w.handler(msg)
	}
}

type WsPrivateWorker struct {
	queue   chan *conn.PrivateMessage
	handler WsPrivateHandler
}

type WsPrivateHandler func(msg *conn.PrivateMessage)

func NewWsPrivateWorker(queue chan *conn.PrivateMessage, handler WsPrivateHandler) *WsPrivateWorker {
	return &WsPrivateWorker{
		queue:   queue,
		handler: handler,
	}
}

func (w *WsPrivateWorker) Run() {
	for msg := range w.queue {
		w.handler(msg)
	}
}

type WsPublicWorker struct {
	queue   chan *conn.PublicMessage
	handler WsPublicHandler
}

type WsPublicHandler func(msg *conn.PublicMessage)

func NewWsPublicWorker(queue chan *conn.PublicMessage, handler WsPublicHandler) *WsPublicWorker {
	return &WsPublicWorker{
		queue:   queue,
		handler: handler,
	}
}

func (w *WsPublicWorker) Run() {
	for msg := range w.queue {
		w.handler(msg)
	}
}

type WsClientPrivateWorker struct {
	queue   chan *conn.ClientPrivateMessage
	handler WsClientPrivateHandler
}

type WsClientPrivateHandler func(msg *conn.ClientPrivateMessage)

func NewWsClientPrivateWorker(queue chan *conn.ClientPrivateMessage, handler WsClientPrivateHandler) *WsClientPrivateWorker {
	return &WsClientPrivateWorker{
		queue:   queue,
		handler: handler,
	}
}

func (w *WsClientPrivateWorker) Run() {
	for msg := range w.queue {
		w.handler(msg)
	}
}

type WsClientPublicWorker struct {
	queue   chan *conn.ClientPublicMessage
	handler WsClientPublicHandler
}

type WsClientPublicHandler func(msg *conn.ClientPublicMessage)

func NewWsClientPublicWorker(queue chan *conn.ClientPublicMessage, handler WsClientPublicHandler) *WsClientPublicWorker {
	return &WsClientPublicWorker{
		queue:   queue,
		handler: handler,
	}
}

func (w *WsClientPublicWorker) Run() {
	for msg := range w.queue {
		w.handler(msg)
	}
}
