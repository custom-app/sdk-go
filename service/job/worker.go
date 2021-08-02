package job

import "github.com/loyal-inform/sdk-go/service/ws/conn"

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
