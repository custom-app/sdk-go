// Package locker - система блокировки с блокировкой по списку чисел.
//
// Может использоваться, например, при необходимости упорядочивания действий всех пользователей (пока у user1 работает какая-то операция,
// user2 тоже может выполнить какую-то операцию, но user1 не может ничего делать до завершения наложившей блокировку операции)
package locker

import (
	"github.com/loyal-inform/sdk-go/logger"
	"sync"
)

// LockSystem - система блокировки на основе Mutex.
//
// Важно создавать LockSystem с помощью NewLockSystem, чтобы были инициализированы все поля
type LockSystem struct {
	m    map[int64]interface{}
	lock sync.Mutex
}

type lockElem struct {
	m      sync.Mutex
	count  int
	system *LockSystem
	key    int64
}

type lockSubset struct {
	set, system *LockSystem
	count       int
	key         int64
}

func NewLockSystem() *LockSystem {
	return &LockSystem{m: map[int64]interface{}{}, lock: sync.Mutex{}}
}

func (s *LockSystem) Lock(values ...int64) {
	if len(values) == 0 {
		logger.Panic("lock empty")
	}
	s.lock.Lock()
	if len(values) == 1 {
		res, ok := s.m[values[0]]
		if !ok {
			res = &lockElem{
				m:      sync.Mutex{},
				count:  0,
				system: s,
				key:    values[0],
			}
			s.m[values[0]] = res
		}
		res.(*lockElem).count++
		s.lock.Unlock()
		res.(*lockElem).m.Lock()
	} else {
		res, ok := s.m[values[0]]
		if !ok {
			res = &lockSubset{
				set:    NewLockSystem(),
				system: s,
				count:  0,
				key:    values[0],
			}
			s.m[values[0]] = res
		}
		res.(*lockSubset).count++
		s.lock.Unlock()
		res.(*lockSubset).set.Lock(values[1:]...)
	}
}

func (s *LockSystem) Unlock(values ...int64) {
	if len(values) == 0 {
		logger.Panic("unlock empty")
	}
	s.lock.Lock()
	if len(values) == 1 {
		res := s.m[values[0]]
		res.(*lockElem).count--
		if res.(*lockElem).count < 1 {
			delete(s.m, values[0])
		}
		s.lock.Unlock()
		res.(*lockElem).m.Unlock()
	} else {
		res := s.m[values[0]]
		res.(*lockSubset).count--
		if res.(*lockSubset).count < 1 {
			delete(s.m, values[0])
		}
		s.lock.Unlock()
		res.(*lockSubset).set.Unlock(values[1:]...)
	}
}

// RWLockSystem - система блокировки на основе RWMutex.
//
// Важно создавать RWLockSystem с помощью NewRWLockSystem, чтобы были инициализированы все поля
type RWLockSystem struct {
	m    map[int64]interface{}
	lock sync.Mutex
}

type rwLockElem struct {
	m      sync.RWMutex
	count  int
	system *RWLockSystem
	key    int64
}

type rwLockSubset struct {
	set, system *RWLockSystem
	count       int
	key         int64
}

func NewRWLockSystem() *RWLockSystem {
	return &RWLockSystem{m: map[int64]interface{}{}, lock: sync.Mutex{}}
}

func (s *RWLockSystem) Lock(values ...int64) {
	if len(values) == 0 {
		logger.Panic("lock empty")
	}
	s.lock.Lock()
	if len(values) == 1 {
		res, ok := s.m[values[0]]
		if !ok {
			res = &rwLockElem{
				m:      sync.RWMutex{},
				count:  0,
				system: s,
				key:    values[0],
			}
			s.m[values[0]] = res
		}
		res.(*rwLockElem).count++
		s.lock.Unlock()
		res.(*rwLockElem).m.Lock()
	} else {
		res, ok := s.m[values[0]]
		if !ok {
			res = &rwLockSubset{
				set:    NewRWLockSystem(),
				system: s,
				count:  0,
				key:    values[0],
			}
			s.m[values[0]] = res
		}
		res.(*rwLockSubset).count++
		s.lock.Unlock()
		res.(*rwLockSubset).set.Lock(values[1:]...)
	}
}

func (s *RWLockSystem) Unlock(values ...int64) {
	if len(values) == 0 {
		logger.Panic("unlock empty")
	}
	s.lock.Lock()
	if len(values) == 1 {
		res := s.m[values[0]]
		res.(*rwLockElem).count--
		if res.(*rwLockElem).count < 1 {
			delete(s.m, values[0])
		}
		s.lock.Unlock()
		res.(*rwLockElem).m.Unlock()
	} else {
		res := s.m[values[0]]
		res.(*rwLockSubset).count--
		if res.(*rwLockSubset).count < 1 {
			delete(s.m, values[0])
		}
		s.lock.Unlock()
		res.(*rwLockSubset).set.Unlock(values[1:]...)
	}
}

func (s *RWLockSystem) RLock(values ...int64) {
	if len(values) == 0 {
		logger.Panic("lock empty")
	}
	s.lock.Lock()
	if len(values) == 1 {
		res, ok := s.m[values[0]]
		if !ok {
			res = &rwLockElem{
				m:      sync.RWMutex{},
				count:  0,
				system: s,
				key:    values[0],
			}
			s.m[values[0]] = res
		}
		res.(*rwLockElem).count++
		s.lock.Unlock()
		res.(*rwLockElem).m.RLock()
	} else {
		res, ok := s.m[values[0]]
		if !ok {
			res = &rwLockSubset{
				set:    NewRWLockSystem(),
				system: s,
				count:  0,
				key:    values[0],
			}
			s.m[values[0]] = res
		}
		res.(*rwLockSubset).count++
		s.lock.Unlock()
		res.(*rwLockSubset).set.Lock(values[1:]...)
	}
}

func (s *RWLockSystem) RUnlock(values ...int64) {
	if len(values) == 0 {
		logger.Panic("unlock empty")
	}
	s.lock.Lock()
	if len(values) == 1 {
		res := s.m[values[0]]
		res.(*rwLockElem).count--
		if res.(*rwLockElem).count < 1 {
			delete(s.m, values[0])
		}
		s.lock.Unlock()
		res.(*rwLockElem).m.RUnlock()
	} else {
		res := s.m[values[0]]
		res.(*rwLockSubset).count--
		if res.(*rwLockSubset).count < 1 {
			delete(s.m, values[0])
		}
		s.lock.Unlock()
		res.(*rwLockSubset).set.Unlock(values[1:]...)
	}
}
