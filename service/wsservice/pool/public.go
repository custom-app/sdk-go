package pool

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/service/ws/conn"
	"github.com/loyal-inform/sdk-go/service/ws/opts"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"net/http"
	"sync"
	"time"
)

// PublicPool - пул публичных соединений
//
// Пул нужен для возможности разрыва всех соединений и сбора всех сообщений в общую очередь,
// а также для возможно рассылки сообщений по подписке
type PublicPool struct {
	conns    map[int64]*conn.ServerPublicConn
	connLock *sync.RWMutex
	options  *opts.ServerPublicConnOptions
	queue    chan *conn.PublicMessage
	timeout  time.Duration
}

// NewPublicPool - создание нового пула с опциями. timeout - таймаут общего буфера, queueSize - его размер
func NewPublicPool(options *opts.ServerPublicConnOptions,
	timeout time.Duration, queueSize int) *PublicPool {
	res := &PublicPool{
		conns:    map[int64]*conn.ServerPublicConn{},
		options:  options,
		connLock: &sync.RWMutex{},
		queue:    make(chan *conn.PublicMessage, queueSize),
		timeout:  timeout,
	}
	return res
}

// AddConnection - функция добавления соединения в пул
func (p *PublicPool) AddConnection(w http.ResponseWriter, r *http.Request) (*conn.ServerPublicConn, error) {
	p.connLock.Lock()
	id := time.Now().UnixNano()
	if _, ok := p.conns[id]; ok {
		return nil, fmt.Errorf("id assign failed")
	}
	p.conns[id] = nil
	p.connLock.Unlock()
	c, err := conn.UpgradePublicServerConn(&websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}, w, r, p.options, nil)
	if err != nil {
		logger.Log("add connection failed: ", err)
		return nil, err
	}
	c.SetConnId(id)
	go func(c *conn.ServerPublicConn) {
		for msg := range c.ReceiveBuf() {
			select {
			case p.queue <- msg:
				break
			case <-time.After(p.timeout):
				c.SendData(&conn.SentMessage{
					Data: p.options.OverflowMsg,
				})
				break
			}
			p.queue <- msg
		}
	}(c)
	return c, nil
}

func (p *PublicPool) onclose(connId int64) {
	p.connLock.Lock()
	delete(p.conns, connId)
	p.connLock.Unlock()
}

// SendOnSubAll - функция отправки сообщения по подписке
func (p *PublicPool) SendOnSubAll(kind structs.SubKind, data proto.Message) {
	p.connLock.RLock()
	for _, c := range p.conns {
		if c.GetSub(kind) {
			go func(c *conn.ServerPublicConn) {
				c.SendData(&conn.SentMessage{
					Data: data,
				})
			}(c)
		}
	}
	p.connLock.RUnlock()
}

// HandleResult - функция обработки результата
//
// В данном случае - только рассылка подписок
func (p *PublicPool) HandleResult(res structs.Result) {
	for _, s := range res.GetSubs() {
		if s != nil {
			p.SendOnSubAll(s.GetKind(), s.GetData())
		}
	}
}

// GetQueue - получение общего буфера сообщений для их дальнейшей обработки
func (p *PublicPool) GetQueue() chan *conn.PublicMessage {
	return p.queue
}

// Close - закрытие всех соединений в пуле и буфера
func (p *PublicPool) Close() {
	p.connLock.Lock()
	for _, c := range p.conns {
		go c.Close()
	}
	p.connLock.Unlock()
	close(p.queue)
}
