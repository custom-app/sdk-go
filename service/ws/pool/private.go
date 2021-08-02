package pool

import (
	"github.com/gorilla/websocket"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/service/ws/conn"
	"github.com/loyal-inform/sdk-go/structs"
	"github.com/loyal-inform/sdk-go/util/locker"
	"google.golang.org/protobuf/proto"
	"net/http"
	"time"
)

type PrivatePool struct {
	pools   map[structs.Role]sameRolePool
	opts    *conn.ServerPrivateConnOptions
	queue   chan *conn.PrivateMessage
	timeout time.Duration
}

type sameRolePool struct {
	lock  *locker.LockSystem
	conns map[int64]orderedConn
}

type orderedConn []*conn.ServerPrivateConn

func NewPrivatePool(opts *conn.ServerPrivateConnOptions, roles []structs.Role,
	timeout time.Duration, queueSize int) (*PrivatePool, error) {
	res := &PrivatePool{
		pools:   map[structs.Role]sameRolePool{},
		opts:    opts,
		queue:   make(chan *conn.PrivateMessage, queueSize),
		timeout: timeout,
	}
	for _, r := range roles {
		res.pools[r] = sameRolePool{conns: map[int64]orderedConn{}, lock: locker.NewLockSystem()}
	}
	opts.Onclose = res.onclose
	return res, nil
}

func (p *PrivatePool) AddConnection(w http.ResponseWriter, r *http.Request) (*conn.ServerPrivateConn, error) {
	c, err := conn.UpgradePrivateServerConn(&websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}, w, r, p.opts)
	if err != nil {
		logger.Log("add connection failed: ", err)
		return nil, err
	}
	acc := c.GetAccount()

	rolePool := p.pools[acc.Role]
	rolePool.lock.Lock(acc.Id)
	defer rolePool.lock.Unlock(acc.Id)

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

	p.pools[acc.Role].conns[acc.Id] = append(p.pools[acc.Role].conns[acc.Id], c)
	go func(c *conn.ServerPrivateConn) {
		for msg := range c.ReceiveBuf() {
			select {
			case p.queue <- msg:
				break
			case <-time.After(p.timeout):
				c.SendBuf() <- p.opts.OverflowMsg
				break
			}
		}
	}(c)
	return c, nil
}

func (p *PrivatePool) onclose(acc *structs.Account, connId int64) {
	rolePool := p.pools[acc.Role]
	rolePool.lock.Lock(acc.Id)
	defer rolePool.lock.Unlock(acc.Id)
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

func (p *PrivatePool) SendOnSubAll(role structs.Role, kind structs.SubKind, id, connId int64, data proto.Message, force bool) {
	for setId, set := range p.pools[role].conns {
		p.pools[role].lock.Lock(setId)
		for _, c := range set {
			acc := c.GetAccount()
			subValue := c.GetSub(kind)
			if (acc.Id != id || c.ConnId() != connId || (acc.Id == id && force)) && subValue {
				go func(c *conn.ServerPrivateConn) {
					logger.Log("sending sub with all to", *acc, c.ConnId(), data)
					c.SendBuf() <- data
				}(c)
			}
		}
		p.pools[role].lock.Unlock(setId)
	}
}

func (p *PrivatePool) SendOnSubReceivers(role structs.Role, kind structs.SubKind, id, connId int64, data proto.Message, force bool, receivers []int64) {
	for _, recId := range receivers {
		p.pools[role].lock.Lock(recId)
		for _, c := range p.pools[role].conns[recId] {
			acc := c.GetAccount()
			subValue := c.GetSub(kind)
			if (acc.Id != id || c.ConnId() != connId || (acc.Id == id && force)) && subValue {
				logger.Log("sending sub with receivers to", *acc, c.ConnId(), data)
				c.SendBuf() <- data
			}
		}
		p.pools[role].lock.Unlock(recId)
	}
}

func (p *PrivatePool) HandleAccountDrop(upd *structs.Account) {
	logger.Info("handling account drop", upd)
	p.pools[upd.Role].lock.Lock(upd.Id)
	defer p.pools[upd.Role].lock.Unlock(upd.Id)
	list := p.pools[upd.Role].conns[upd.Id]
	p.pools[upd.Role].conns[upd.Id] = nil
	for _, v := range list {
		go v.Close()
	}
}

type SenderInfo struct {
	Role  structs.Role
	Id    int64
	Index int
}

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

func (p *PrivatePool) GetQueue() chan *conn.PrivateMessage {
	return p.queue
}

func (p *PrivatePool) Close() {
	for _, v := range p.pools {
		for _, list := range v.conns {
			for _, c := range list {
				c.Close()
			}
		}
	}
	close(p.queue)
}
