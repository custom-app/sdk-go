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
	"sync/atomic"
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

type ClientPublicMessage struct {
	Data []byte
	Conn *ClientPublicConn
}

// ClientPublicConn - клиентское соединение с сервером без авторизации
type ClientPublicConn struct {
	*Conn
	opts        *ClientPublicConnOptions
	url         string
	header      http.Header
	needRestart bool
	receiveBuf  chan *ClientPublicMessage

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
	res := &ClientPublicConn{
		opts:        opts,
		Conn:        c,
		url:         url,
		header:      header,
		needRestart: true,
		subData:     map[structs.SubKind]clientSubData{},
		subDataLock: &sync.RWMutex{},
	}
	if needStart {
		res.start()
		c.receiveBuf = make(chan *ReceivedMessage, opts.ReceiveBufSize)
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
	c.SendData(&SentMessage{
		Data: req,
	})
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
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, allCodes...) {
				logger.Log("normal close")
			} else if websocket.IsUnexpectedCloseError(err, allCodes...) {
				logger.Log("unexpected close")
			} else {
				logger.Log("ws read message err: ", err)
			}
			break
		}
		if !c.IsAlive() {
			break
		}
		select {
		case c.receiveBuf <- &ClientPublicMessage{
			Conn: c,
			Data: msg,
		}:
			break
		case <-time.After(c.opts.ReceiveBufTimeout):
			c.sendOverflowMessage()
		}
	}
	if c.IsAlive() {
		c.pingCloseCh <- true
		c.sendCloseCh <- true
	}
	c.restart()
}

func (c *ClientPublicConn) listenSend() {
L:
	for {
		select {
		case data := <-c.sendBuf:
			if data != nil && data.Data != nil {
				c.sendProto(data.Data)
			}
			break
		case <-c.sendCloseCh:
			break L
		}
	}
}

func (c *ClientPublicConn) Close() {
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.wg.Wait()
		c.close()
	}
}

func (c *ClientPublicConn) close() {
	c.needRestart = false
	c.subDataLock.Lock()
	for _, v := range c.subData {
		close(v.confirm)
		v.confirm = nil
	}
	c.subDataLock.Unlock()
	if c.receiveBuf != nil {
		close(c.receiveBuf)
	}
	c.needRestart = false
	c.Conn.close()
}

type ClientPrivateConnOptions struct {
	*ClientPublicConnOptions
	AuthTimeout time.Duration
}

type ClientPrivateMessage struct {
	Data []byte
	Conn *ClientPrivateConn
}

type ClientPrivateConn struct {
	*ClientPublicConn
	opts                *ClientPrivateConnOptions
	authData            proto.Message
	authSuccessChan     chan bool
	authSuccessChanLock *sync.RWMutex
	receiveBuf          chan *ClientPrivateMessage
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
		opts:                opts,
		ClientPublicConn:    c,
		authData:            data,
		authSuccessChan:     make(chan bool),
		authSuccessChanLock: &sync.RWMutex{},
		receiveBuf:          make(chan *ClientPrivateMessage, opts.ReceiveBufSize),
	}
	res.start()
	return res, nil
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
		opts:                opts,
		ClientPublicConn:    c,
		authSuccessChan:     make(chan bool),
		authSuccessChanLock: &sync.RWMutex{},
		receiveBuf:          make(chan *ClientPrivateMessage, opts.ReceiveBufSize),
	}
	res.start()
	return res, nil
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
		opts:                opts,
		ClientPublicConn:    c,
		authSuccessChan:     make(chan bool),
		authSuccessChanLock: &sync.RWMutex{},
		receiveBuf:          make(chan *ClientPrivateMessage, opts.ReceiveBufSize),
	}
	res.start()
	return res, nil
}

func (c *ClientPrivateConn) start() {
	go c.pingPong()
	go c.listenReceive()
	go c.listenSend()
}

func (c *ClientPrivateConn) Auth() error {
	if c.authData != nil {
		c.SendData(&SentMessage{
			Data: c.authData,
		})
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
		if err := c.Auth(); err != nil {
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
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, allCodes...) {
				logger.Log("client private conn normal close")
			} else if websocket.IsUnexpectedCloseError(err, allCodes...) {
				logger.Log("client private conn unexpected close")
			} else {
				logger.Log("client private conn ws read message err: ", err)
			}
			break
		}
		if !c.IsAlive() {
			break
		}
		select {
		case c.receiveBuf <- &ClientPrivateMessage{
			Conn: c,
			Data: msg,
		}:
			break
		case <-time.After(c.opts.ReceiveBufTimeout):
			c.sendOverflowMessage()
		}
	}
	if c.IsAlive() {
		c.pingCloseCh <- true
		c.sendCloseCh <- true
	}
	c.restart()
}

func (c *ClientPrivateConn) listenSend() {
L:
	for {
		select {
		case data := <-c.sendBuf:
			if data != nil && data.Data != nil {
				c.sendProto(data.Data)
			}
			break
		case <-c.sendCloseCh:
			break L
		}
	}
}

func (c *ClientPrivateConn) ReceiveBuf() chan *ClientPrivateMessage {
	return c.receiveBuf
}

func (c *ClientPrivateConn) Close() {
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.wg.Wait()
		c.close()
	}
}

func (c *ClientPrivateConn) close() {
	c.authSuccessChanLock.Lock()
	close(c.receiveBuf)
	close(c.authSuccessChan)
	c.authSuccessChan = nil
	c.authSuccessChanLock.Unlock()
	c.needRestart = false
	c.ClientPublicConn.Close()
}
