package pool

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/service/ws/conn"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"net/http"
	"sync"
	"time"
)

type PublicPool struct {
	conns    map[int64]*conn.ServerPublicConn
	connLock *sync.RWMutex
	opts     *conn.ServerPublicConnOptions
}

func NewPublicPool(opts *conn.ServerPublicConnOptions) (*PublicPool, error) {
	res := &PublicPool{
		conns:    map[int64]*conn.ServerPublicConn{},
		opts:     opts,
		connLock: &sync.RWMutex{},
	}
	opts.Onclose = res.onclose
	return res, nil
}

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
	}, w, r, p.opts)
	if err != nil {
		logger.Log("add connection failed: ", err)
		return nil, err
	}
	c.SetConnId(id)
	return c, nil
}

func (p *PublicPool) onclose(connId int64) {
	p.connLock.Lock()
	delete(p.conns, connId)
	p.connLock.Unlock()
}

func (p *PublicPool) SendOnSubAll(kind structs.SubKind, data proto.Message) {
	p.connLock.RLock()
	for _, c := range p.conns {
		if c.GetSub(kind) {
			go func(c *conn.ServerPublicConn) {
				c.SendBuf() <- data
			}(c)
		}
	}
	p.connLock.RUnlock()
}

func (p *PublicPool) HandleResult(res structs.Result) {
	for _, s := range res.GetSubs() {
		if s != nil {
			p.SendOnSubAll(s.GetKind(), s.GetData())
		}
	}
}

func (p *PublicPool) Close() {
	for _, c := range p.conns {
		c.Close()
	}
}
