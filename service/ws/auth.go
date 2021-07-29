package ws

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/service/job"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/encoding/protojson"
	"sync"
	"time"
)

func (c *Conn) SelfAuth(login, password string) error {
	handler := c.handler
	defer func() {
		c.handler = handler
	}()
	res := make(chan error)
	deadline := time.After(config.SelfAuthTimeout())

	wg := &sync.WaitGroup{}
	c.handler = func(conn *Conn, acc *structs.Account, bytes []byte) *structs.Result {
		wg.Add(1)
		var msg coffeepb.Response
		if err := proto.Unmarshal(bytes, &msg); err != nil {
			res <- err
			wg.Done()
			return nil
		}
		data := msg.GetAuth()
		if data == nil {
			res <- fmt.Errorf("receive nil auth: %v", msg)
			wg.Done()
			return nil
		}
		res <- nil
		wg.Done()
		return nil
	}

	go c.listenReceive()
	go c.pingPong()

	req := &coffeepb.Request{
		Id: 1,
		Data: &coffeepb.Request_Auth{Auth: &coffeepb.AuthRequest{
			Login:    login,
			Password: password,
			Platform: coffeepb.Platform_Frontend,
			Version:  config.GetVersions(coffeepb.Platform_Frontend)[0],
		}},
	}
	c.SendProto(req)

	select {
	case err := <-res:
		if err != nil {
			return err
		}
		c.handler = nil // stop receive
		wg.Wait()       // wait before close
		close(res)
		return nil
	case <-deadline:
		c.handler = nil // stop receive
		wg.Wait()       // wait before close
		close(res)
		return job.TimeoutError
	}
}

func (c *Conn) AuthWait(authOptions *AuthOptions) error {
	handler := c.handler
	res := make(chan *structs.Account, 5)
	deadline := time.After(authOptions.Timeout)
	setupWait, setupFinished := make(chan bool, 2), false
	authMutex := &sync.Mutex{}
	c.handler = func(conn *Conn, acc *structs.Account, bytes []byte) structs.Result {
		authMutex.Lock() // одновременно обслуживается только один запрос
		defer authMutex.Unlock()
		if setupFinished {
			return nil
		}
		acc, resp := authOptions.RequestAuthFunc(c, bytes)
		if acc != nil {
			res <- acc

			// это делается здесь, чтобы не было задержки между успешной авторизацией и сетапом корректного хендлера
			_, ok := <-setupWait // не ok - отвалился дедлайн до успешной авторизации и канал был закрыт
			if ok {
				close(setupWait)
			}
		}
		return &structs.DefaultResult{
			Response: resp,
		}
	}

	go c.listenReceive()
	go c.pingPong()

	select {
	case data := <-res:
		c.handler = nil // stop receive
		setupFinished = true
		close(res)
		c.SetAccount(data)
		c.handler = handler
		setupWait <- true
		return nil
	case <-deadline:
		c.handler = nil  // stop receive
		close(setupWait) // закрываем канал, чтобы находящаяся в процессе успешная авторизация не сделала дедлок
		authMutex.Lock() // блокируем, после этого максимально в res упадет одно значение, поэтому блокировки не будет
		setupFinished = true
		authMutex.Unlock()
		_, resp := authOptions.ErrorMapper(job.TimeoutError)
		c.SendProto(resp)
		close(res)
		return job.TimeoutError
	}
}

