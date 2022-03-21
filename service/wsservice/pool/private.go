// Package pool - пакет с реализаций пула публичных и авторизованных соединений
package pool

import (
	"github.com/custom-app/sdk-go/logger"
	"github.com/custom-app/sdk-go/service/wsservice/conn"
	"github.com/custom-app/sdk-go/service/wsservice/opts"
	"github.com/custom-app/sdk-go/structs"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"net/http"
	"sync"
	"time"
)

// PrivatePool - пул авторизованных соединений
//
// Пул нужен для возможности разрыва соединений и сбора всех сообщений в общую очередь,
// а также для возможно рассылки сообщений по подписке
type PrivatePool struct {
	pools   map[structs.Role]sameRolePool
	options *opts.ServerPrivateConnOptions
	queue   chan *conn.PrivateMessage
	timeout time.Duration
}

type sameRolePool struct {
	lock  *sync.RWMutex
	conns map[int64]orderedConn
}

type orderedConn []*conn.ServerPrivateConn

// NewPrivatePool - создание нового пула с опциями. timeout - таймаут общего буфера, queueSize - его размер,
// roles - список ролей в системе для создания пулов соединений пользователей с одинаковыми ролями
func NewPrivatePool(options *opts.ServerPrivateConnOptions, roles []structs.Role,
	timeout time.Duration, queueSize int) (*PrivatePool, error) {
	if err := opts.FillServerPrivateOptions(options); err != nil {
		return nil, err
	}
	res := &PrivatePool{
		pools:   map[structs.Role]sameRolePool{},
		options: options,
		queue:   make(chan *conn.PrivateMessage, queueSize),
		timeout: timeout,
	}
	for _, r := range roles {
		res.pools[r] = sameRolePool{conns: map[int64]orderedConn{}, lock: &sync.RWMutex{}}
	}
	return res, nil
}

// AddConnection - функция добавления соединения в пул
func (p *PrivatePool) AddConnection(w http.ResponseWriter, r *http.Request) (*conn.ServerPrivateConn, error) {
	c, err := conn.UpgradePrivateServerConn(&websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}, w, r, p.options, p.onauth, p.onclose)
	if err != nil {
		logger.Log("add connection failed: ", err)
		return nil, err
	}
	go func(c *conn.ServerPrivateConn) {
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
		}
	}(c)
	return c, nil
}

func (p *PrivatePool) onauth(c *conn.ServerPrivateConn) {
	acc := c.GetAccount()

	rolePool := p.pools[acc.Role]
	rolePool.lock.Lock()
	defer rolePool.lock.Unlock()

	// connection connId is into [1, +inf)
	list := rolePool.conns[acc.Id]
	var maxIndex int64
	for _, cn := range list {
		if cn.ConnId() > maxIndex {
			maxIndex = cn.ConnId()
		}
	}
	// so used slice will always have at least one false element
	// empty conn list case is also handled
	used := make([]bool, maxIndex+1)
	for _, cn := range list {
		used[cn.ConnId()-1] = true
	}
	for i, v := range used {
		if !v {
			c.SetConnId(int64(i + 1))
		}
	}

	rolePool.conns[acc.Id] = append(rolePool.conns[acc.Id], c)
}

func (p *PrivatePool) onclose(acc *structs.Account, connId int64) {
	rolePool := p.pools[acc.Role]
	rolePool.lock.Lock()
	defer rolePool.lock.Unlock()
	index := -1
	for i, c := range rolePool.conns[acc.Id] {
		if c.ConnId() == connId {
			index = i
			break
		}
	}
	if index == -1 {
		logger.Info("somehow not found connection with given id ind list")
		return
	}
	rolePool.conns[acc.Id] = append(rolePool.conns[acc.Id][:index], rolePool.conns[acc.Id][index+1:]...)
}

// SendOnSubAll - функция отправки сообщения по подписке
func (p *PrivatePool) SendOnSubAll(role structs.Role, kind structs.SubKind, id, connId int64, data proto.Message, force bool) {
	p.pools[role].lock.RLock()
	for _, set := range p.pools[role].conns {
		for _, c := range set {
			acc := c.GetAccount()
			subValue := c.GetSub(kind)
			if (acc.Id != id || c.ConnId() != connId || (acc.Id == id && force)) && subValue {
				logger.Log("sending sub with all to", *acc, c.ConnId(), data)
				c.SendData(&conn.SentMessage{
					Data: data,
				})
			}
		}
	}
	p.pools[role].lock.RUnlock()
}

// SendOnSubReceivers - функция отправки сообщения по подписке с фильтрацией по id
func (p *PrivatePool) SendOnSubReceivers(role structs.Role, kind structs.SubKind, id, connId int64, data proto.Message, force bool, receivers []int64) {
	p.pools[role].lock.RLock()
	for _, recId := range receivers {
		for _, c := range p.pools[role].conns[recId] {
			acc := c.GetAccount()
			subValue := c.GetSub(kind)
			if (acc.Id != id || c.ConnId() != connId || (acc.Id == id && force)) && subValue {
				logger.Log("sending sub with receivers to", *acc, c.ConnId(), data)
				c.SendData(&conn.SentMessage{
					Data: data,
				})
			}
		}
	}
	p.pools[role].lock.RUnlock()
}

// HandleAccountDrop - разрыв всех соединений пользователя
func (p *PrivatePool) HandleAccountDrop(upd *structs.Account) {
	logger.Info("handling account drop", upd)
	p.pools[upd.Role].lock.Lock()
	defer p.pools[upd.Role].lock.Unlock()
	list := p.pools[upd.Role].conns[upd.Id]
	p.pools[upd.Role].conns[upd.Id] = nil
	for _, v := range list {
		go v.Close()
	}
}

// HandleResult - функция обработки результата
//
// В данном случае - рассылка подписок и разрыв соединений
func (p *PrivatePool) HandleResult(res structs.Result, acc *structs.Account, connId int64) {
	for _, s := range res.GetSubs() {
		if s != nil {
			logger.Log("sub: ", s)
			for k := range s.GetAll() {
				if acc != nil && k == acc.Role {
					p.SendOnSubAll(k, s.GetKind(), acc.Id, connId, s.GetData(), s.GetForce())
				} else {
					p.SendOnSubAll(k, s.GetKind(), 0, 0, s.GetData(), s.GetForce())
				}
			}
			for k, v := range s.GetReceivers() {
				if acc != nil && k == acc.Role {
					p.SendOnSubReceivers(k, s.GetKind(), acc.Id, connId, s.GetData(), s.GetForce(), v)
				} else {
					p.SendOnSubReceivers(k, s.GetKind(), 0, 0, s.GetData(), s.GetForce(), v)
				}
			}
		}
	}
	for _, upd := range res.GetAccountsToDrop() {
		p.HandleAccountDrop(upd)
	}
}

// GetQueue - получение общего буфера сообщений для их дальнейшей обработки
func (p *PrivatePool) GetQueue() chan *conn.PrivateMessage {
	return p.queue
}

// Close - закрытие всех соединений в пуле и буфера
func (p *PrivatePool) Close() {
	for _, v := range p.pools {
		v.lock.Lock()
		for id := range v.conns {
			for _, c := range v.conns[id] {
				go c.Close()
			}
		}
		v.lock.Unlock()
	}
	close(p.queue)
}
