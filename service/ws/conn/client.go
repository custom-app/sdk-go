package conn

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/structs"
	"github.com/loyal-inform/sdk-go/util/consts"
	"google.golang.org/protobuf/proto"
	"net/http"
	"sync"
	"time"
)

const (
	defaultRetryTimeout = 250 * time.Millisecond
	defaultRetryLimit   = 20
	defaultSubTimeout   = 10 * time.Second
	defaultAuthTimeout  = 10 * time.Second
)

var (
	HasSubErr      = errors.New("already has sub")
	SubTimeoutErr  = errors.New("sub timeout")
	NoSubErr       = errors.New("sub doesn't exist")
	AuthTimeoutErr = errors.New("auth timeout")
)

type clientSubData struct {
	req     proto.Message
	confirm chan bool
}

type ClientPublicConnOptions struct {
	*Options
	RetryLimit              int
	RetryPeriod, SubTimeout time.Duration
}

// ClientPublicConn - клиентское соединение с сервером без авторизации
type ClientPublicConn struct {
	*Conn
	opts        *ClientPublicConnOptions
	url         string
	header      http.Header
	needRestart bool

	subData     map[structs.SubKind]clientSubData
	subDataLock *sync.RWMutex
}

func fillClientPublicOptions(opts *ClientPublicConnOptions) error {
	if opts.Options == nil {
		return OptsRequiredErr
	}
	if err := fillOpts(opts.Options); err != nil {
		return err
	}
	if opts.RetryLimit == 0 {
		opts.RetryLimit = defaultRetryLimit
	}
	if opts.RetryPeriod == 0 {
		opts.RetryPeriod = defaultRetryTimeout
	}
	if opts.SubTimeout == 0 {
		opts.SubTimeout = defaultSubTimeout
	}
	return nil
}

func newClientConn(url string, header http.Header, opts *ClientPublicConnOptions, needStart bool) (*ClientPublicConn, error) {
	if err := fillClientPublicOptions(opts); err != nil {
		return nil, err
	}
	dialer := &websocket.Dialer{}
	var (
		conn *websocket.Conn
		err  error
	)
	for i := 0; i < opts.RetryLimit; i++ {
		conn, _, err = dialer.Dial(url, header)
		if err != nil {
			conn = nil
			time.Sleep(opts.RetryPeriod)
			continue
		}
		break
	}
	opts.ContentType = header.Get(consts.HeaderContentType)
	c, err := newConn(conn, opts.Options, false)
	if err != nil {
		return nil, err
	}
	c.receiveBuf = make(chan *Message, opts.ReceiveBufSize)
	res := &ClientPublicConn{
		Conn:        c,
		url:         url,
		header:      header,
		needRestart: true,
		subData:     map[structs.SubKind]clientSubData{},
		subDataLock: &sync.RWMutex{},
	}
	if needStart {
		res.start()
	}
	return res, nil
}

func NewClientConn(url string, header http.Header, opts *ClientPublicConnOptions) (*ClientPublicConn, error) {
	return newClientConn(url, header, opts, true)
}

func (c *ClientPublicConn) start() {
	go c.pingPong()
	go c.listenReceive()
	go c.listenSend()
}

func (c *ClientPublicConn) Sub(kind structs.SubKind, req proto.Message) error {
	c.subDataLock.Lock()
	if _, ok := c.subData[kind]; ok {
		c.subDataLock.Unlock()
		return HasSubErr
	}
	ch := make(chan bool)
	c.subData[kind] = clientSubData{
		req:     req,
		confirm: ch,
	}
	c.subDataLock.Unlock()
	return c.sub(kind, req, ch)
}

func (c *ClientPublicConn) sub(kind structs.SubKind, req proto.Message, confirmCh chan bool) error {
	c.sendBuf <- req
	select {
	case _, _ = <-confirmCh:
		c.SetSub(kind, true)
		return nil
	case <-time.After(c.opts.SubTimeout):
		return SubTimeoutErr
	}
}

func (c *ClientPublicConn) SubConfirm(kind structs.SubKind) error {
	c.subDataLock.RLock()
	if v, ok := c.subData[kind]; !ok {
		c.subDataLock.RUnlock()
		return NoSubErr
	} else {
		if v.confirm != nil {
			v.confirm <- true
		}
	}
	c.subDataLock.RUnlock()
	return nil
}

func (c *ClientPublicConn) restart() {
	for {
		if !c.needRestart {
			break
		}
		dialer := &websocket.Dialer{}
		conn, _, err := dialer.Dial(c.url, c.header)
		if err != nil {
			logger.Info("restart dial connection failed", err)
			time.Sleep(c.opts.RetryPeriod)
			continue
		}
		c.conn = conn
		c.start()
		c.restartSubs()
		break
	}
}

func (c *ClientPublicConn) restartSubs() {
	c.subDataLock.RLock()
	wg := &sync.WaitGroup{}
	for k, v := range c.subscriptions {
		if v {
			wg.Add(1)
			req, confirmCh := c.subData[k].req, c.subData[k].confirm
			go func(kind structs.SubKind, req proto.Message, confirmCh chan bool) {
				if err := c.sub(k, req, confirmCh); err != nil {
					logger.Info("restart re sub failed", k, err)
					c.SetSub(k, false)
				}
			}(k, req, confirmCh)
		}
	}
	c.subDataLock.RUnlock()
	wg.Wait()
}

func (c *ClientPublicConn) listenReceive() {
	c.Conn.listenReceive()
	if c.IsAlive() {
		c.pingCloseCh <- true
		c.sendCloseCh <- true
	}
	c.restart()
}

func (c *ClientPublicConn) Close() {
	c.needRestart = false
	c.subDataLock.Lock()
	for _, v := range c.subData {
		close(v.confirm)
		v.confirm = nil
	}
	c.subDataLock.Unlock()
	c.Conn.Close()
}

type ClientPrivateConnOptions struct {
	*ClientPublicConnOptions
	AuthTimeout time.Duration
}

type ClientPrivateConn struct {
	*ClientPublicConn
	opts                *ClientPrivateConnOptions
	authData            proto.Message
	authSuccessChan     chan bool
	authSuccessChanLock *sync.RWMutex
}

func fillClientPrivateConnOptions(opts *ClientPrivateConnOptions) error {
	if opts.ClientPublicConnOptions == nil {
		return OptsRequiredErr
	}
	if err := fillClientPublicOptions(opts.ClientPublicConnOptions); err != nil {
		return err
	}
	if opts.AuthTimeout == 0 {
		opts.AuthTimeout = defaultAuthTimeout
	}
	return nil
}

func NewClientPrivateConnWithRequest(url string, data proto.Message, opts *ClientPrivateConnOptions) (*ClientPrivateConn, error) {
	if err := fillClientPrivateConnOptions(opts); err != nil {
		return nil, err
	}
	header := http.Header{}
	header.Set(consts.HeaderContentType, opts.ContentType)
	c, err := newClientConn(url, header, opts.ClientPublicConnOptions, false)
	if err != nil {
		return nil, err
	}
	res := &ClientPrivateConn{
		ClientPublicConn:    c,
		authData:            data,
		authSuccessChan:     make(chan bool),
		authSuccessChanLock: &sync.RWMutex{},
	}
	res.start()
	return res, res.auth()
}

func NewClientPrivateConnWithBasic(url, login, pass string, opts *ClientPrivateConnOptions) (*ClientPrivateConn, error) {
	if err := fillClientPrivateConnOptions(opts); err != nil {
		return nil, err
	}
	header := http.Header{}
	header.Set(consts.HeaderContentType, opts.ContentType)
	header.Set(consts.AuthHeader,
		fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", login, pass)))))
	c, err := newClientConn(url, header, opts.ClientPublicConnOptions, false)
	if err != nil {
		return nil, err
	}
	res := &ClientPrivateConn{
		ClientPublicConn:    c,
		authSuccessChan:     make(chan bool),
		authSuccessChanLock: &sync.RWMutex{},
	}
	res.start()
	return res, res.auth()
}

func NewClientPrivateConnWithToken(url, token string, opts *ClientPrivateConnOptions) (*ClientPrivateConn, error) {
	if err := fillClientPrivateConnOptions(opts); err != nil {
		return nil, err
	}
	header := http.Header{}
	header.Set(consts.HeaderContentType, opts.ContentType)
	header.Set(consts.AuthHeader, fmt.Sprintf("Bearer %s", token))
	c, err := newClientConn(url, header, opts.ClientPublicConnOptions, false)
	if err != nil {
		return nil, err
	}
	res := &ClientPrivateConn{
		ClientPublicConn:    c,
		authSuccessChan:     make(chan bool),
		authSuccessChanLock: &sync.RWMutex{},
	}
	res.start()
	return res, res.auth()
}

func (c *ClientPrivateConn) start() {
	go c.pingPong()
	go c.listenReceive()
	go c.listenSend()
}

func (c *ClientPrivateConn) auth() error {
	if c.authData != nil {
		c.sendBuf <- c.authData
	}
	select {
	case _, _ = <-c.authSuccessChan:
		return nil
	case <-time.After(c.opts.AuthTimeout):
		return AuthTimeoutErr
	}
}

func (c *ClientPrivateConn) AuthConfirm() error {
	c.authSuccessChanLock.RLock()
	c.authSuccessChan <- true
	c.authSuccessChanLock.RUnlock()
	return nil
}

func (c *ClientPrivateConn) restart() {
	for {
		if !c.needRestart {
			break
		}
		dialer := &websocket.Dialer{}
		conn, _, err := dialer.Dial(c.url, c.header)
		if err != nil {
			logger.Info("restart dial connection failed", err)
			time.Sleep(c.opts.RetryPeriod)
			continue
		}
		c.conn = conn
		c.start()
		if err := c.auth(); err != nil {
			logger.Info("restart auth failed", err)
			if err := c.conn.Close(); err != nil {
				logger.Info("restart close after auth failed err", err)
			}
			continue
		}
		c.restartSubs()
		break
	}
}

func (c *ClientPrivateConn) listenReceive() {
	c.Conn.listenReceive()
	if c.IsAlive() {
		c.pingCloseCh <- true
		c.sendCloseCh <- true
	}
	c.restart()
}

func (c *ClientPrivateConn) Close() {
	c.authSuccessChanLock.Lock()
	close(c.authSuccessChan)
	c.authSuccessChan = nil
	c.authSuccessChanLock.Unlock()
	c.ClientPublicConn.Close()
}
