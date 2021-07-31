package pool

import (
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/service/ws/conn"
	"github.com/loyal-inform/sdk-go/structs"
	"github.com/loyal-inform/sdk-go/util/locker"
	"google.golang.org/protobuf/proto"
	"net/http"
)

type Pool struct {
	pools       map[structs.Role]sameRolePool
	authOptions *conn.AuthOptions
}

type sameRolePool struct {
	lock  *locker.LockSystem
	conns map[int64]orderedConn
}

type orderedConn []*conn.ClientPrivateConn

func NewPool(authOptions *conn.AuthOptions, roles []structs.Role) (*Pool, error) {
	res := &Pool{
		pools:       map[structs.Role]sameRolePool{},
		authOptions: authOptions,
	}
	for _, r := range roles {
		res.pools[r] = sameRolePool{conns: map[int64]orderedConn{}, lock: locker.NewLockSystem()}
		res.pools[r] = sameRolePool{conns: map[int64]orderedConn{}, lock: locker.NewLockSystem()}
		res.pools[r] = sameRolePool{conns: map[int64]orderedConn{}, lock: locker.NewLockSystem()}
		res.pools[r] = sameRolePool{conns: map[int64]orderedConn{}, lock: locker.NewLockSystem()}
		res.pools[r] = sameRolePool{conns: map[int64]orderedConn{}, lock: locker.NewLockSystem()}
	}
	return res, nil
}

func (p *Pool) AddConnection(w http.ResponseWriter, r *http.Request, handler conn.MessageHandler) *conn.Conn {
	c, err := conn.NewConn(w, r, handler, p.onclose, p.authOptions)
	if err != nil {
		logger.Log("add connection failed: ", err)
		return nil
	}
	c.pool = p

	c.accLock.RLock()
	acc := c.GetAccount()
	c.accLock.RUnlock()

	rolePool := p.pools[acc.Role]
	rolePool.lock.Lock(acc.Id)
	defer rolePool.lock.Unlock(acc.Id)

	// connection connId is into [1, +inf)
	list := rolePool.conns[acc.Id]
	var maxIndex int
	for _, conn := range list {
		if conn.connId > maxIndex {
			maxIndex = conn.connId
		}
	}
	// so used slice will always have at least one false element
	// empty conn list case is also handled
	used := make([]bool, maxIndex+1)
	for _, conn := range list {
		used[conn.connId-1] = true
	}
	for i, v := range used {
		if !v {
			c.connId = i + 1
		}
	}

	p.pools[acc.Role].conns[acc.Id] = append(p.pools[acc.Role].conns[acc.Id], c)
	return c
}

func (p *Pool) onclose(acc *structs.Account, conn *conn.Conn) {
	rolePool := p.pools[acc.Role]
	rolePool.lock.Lock(acc.Id)
	defer rolePool.lock.Unlock(acc.Id)
	index := -1
	for i, c := range rolePool.conns[acc.Id] {
		if c.connId == conn.connId {
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

func (p *Pool) SendOnSubAll(role structs.Role, kind structs.SubKind, id int64, connId int, data proto.Message, force bool) {
	for setId, set := range p.pools[role].conns {
		p.pools[role].lock.Lock(setId)
		for _, c := range set {
			acc := c.GetAccount()
			c.subLock.RLock()
			if (acc.Id != id || c.connId != connId || (acc.Id == id && force)) && c.subscriptions[kind] {
				logger.Log("sending sub with all to", *acc, c.connId, data)
				c.SendProto(data)
			}
			c.subLock.RUnlock()
		}
		p.pools[role].lock.Unlock(setId)
	}
}

func (p *Pool) SendOnSubReceivers(role structs.Role, kind structs.SubKind, id int64, connId int, data proto.Message, force bool, receivers []int64) {
	for _, recId := range receivers {
		p.pools[role].lock.Lock(recId)
		for _, c := range p.pools[role].conns[recId] {
			acc := c.GetAccount()
			c.subLock.RLock()
			if (acc.Id != id || c.connId != connId || (acc.Id == id && force)) && c.subscriptions[kind] {
				logger.Log("sending sub with receivers to", *acc, c.connId, data)
				c.SendProto(data)
			}
			c.subLock.RUnlock()
		}
		p.pools[role].lock.Unlock(recId)
	}
}

func (p *Pool) HandleAccountDrop(upd *structs.Account) {
	logger.Info("handling account drop", upd)
	p.pools[upd.Role].lock.Lock(upd.Id)
	defer p.pools[upd.Role].lock.Unlock(upd.Id)
	list := p.pools[upd.Role].conns[upd.Id]
	p.pools[upd.Role].conns[upd.Id] = nil
	for _, v := range list {
		v.onclose = nil
		go v.Shutdown()
	}
}

type SenderInfo struct {
	Role  structs.Role
	Id    int64
	Index int
}

func (p *Pool) HandleResult(res structs.Result, acc *SenderInfo) {
	for _, s := range res.GetSubs() {
		if s != nil {
			logger.Log("sub: ", s)
			for k := range s.GetAll() {
				if acc != nil && k == acc.Role {
					p.SendOnSubAll(k, s.GetKind(), acc.Id, acc.Index, s.GetData(), s.GetForce())
				} else {
					p.SendOnSubAll(k, s.GetKind(), 0, 0, s.GetData(), s.GetForce())
				}
			}
			for k, v := range s.GetReceivers() {
				if acc != nil && k == acc.Role {
					p.SendOnSubReceivers(k, s.GetKind(), acc.Id, acc.Index, s.GetData(), s.GetForce(), v)
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

func (p *Pool) Close() {
	for _, v := range p.pools {
		for _, list := range v.conns {
			for _, c := range list {
				c.Close(false, true)
			}
		}
	}
}
