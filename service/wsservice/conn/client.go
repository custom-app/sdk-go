package conn

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/service/wsservice/opts"
	"github.com/loyal-inform/sdk-go/structs"
	"github.com/loyal-inform/sdk-go/util/consts"
	"google.golang.org/protobuf/proto"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
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

// ClientPublicMessage - сообщения, полученные через клиентское публичное соединение
type ClientPublicMessage struct {
	Data []byte
	Conn *ClientPublicConn
}

// ClientPublicConn - клиентское соединение с сервером без авторизации.
//
// Использовать для клиентских соединений.
//
// Процесс восстановления соединения при отключении будет происходить автоматически, если установлены соответствующие
// опции. Подписки так же будут восстановлены.
type ClientPublicConn struct {
	*Conn
	options         *opts.ClientPublicConnOptions
	url             string
	header          http.Header
	needRestart     bool
	needRestartLock *sync.RWMutex
	receiveBuf      chan *ClientPublicMessage

	subData     map[structs.SubKind]clientSubData
	subDataLock *sync.RWMutex
}

func newClientConn(url string, header http.Header, options *opts.ClientPublicConnOptions, needStart bool) (*ClientPublicConn, error) {
	if err := opts.FillClientPublicOptions(options); err != nil {
		return nil, err
	}
	dialer := &websocket.Dialer{}
	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		if options.NeedRestart {
			for i := 0; i < options.RetryLimit; i++ {
				conn, _, err = dialer.Dial(url, header)
				if err != nil {
					conn = nil
					time.Sleep(options.RetryPeriod)
					continue
				}
				break
			}
			if err != nil && conn == nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	options.ContentType = header.Get(consts.HeaderContentType)
	c, err := newConn(conn, options.Options, false)
	if err != nil {
		return nil, err
	}
	res := &ClientPublicConn{
		options:         options,
		Conn:            c,
		url:             url,
		header:          header,
		needRestart:     options.NeedRestart,
		subData:         map[structs.SubKind]clientSubData{},
		subDataLock:     &sync.RWMutex{},
		needRestartLock: &sync.RWMutex{},
	}
	if needStart {
		res.start()
		c.receiveBuf = make(chan *ReceivedMessage, options.ReceiveBufSize)
	}
	return res, nil
}

// NewClientConn - подключение к серверу и создание ClientPublicConn
func NewClientConn(url string, header http.Header, options *opts.ClientPublicConnOptions) (*ClientPublicConn, error) {
	if options.FillVersion != nil {
		options.FillVersion(header)
	}
	return newClientConn(url, header, options, true)
}

func (c *ClientPublicConn) start() {
	go c.pingPong()
	go c.listenReceive()
	go c.listenSend()
}

// Sub - подписка на топик kind
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
	case <-time.After(c.options.SubTimeout):
		return SubTimeoutErr
	}
}

// SubConfirm - подтверждение успешного ответа на подписку (вызывается обработчиком очереди сообщений,
// то есть ответственность за вызов лежит на пользователе SDK)
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
		if !c.needRestartSafe() {
			break
		}
		dialer := &websocket.Dialer{}
		conn, _, err := dialer.Dial(c.url, c.header)
		if err != nil {
			logger.Info("restart dial connection failed", err)
			time.Sleep(c.options.RetryPeriod)
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
	var subKinds []structs.SubKind
	for k, v := range c.subscriptions {
		if v {
			subKinds = append(subKinds, k)
		}
	}
	c.subDataLock.RUnlock()
	for _, k := range subKinds {
		req, confirmCh := c.subData[k].req, c.subData[k].confirm
		go func(k structs.SubKind, req proto.Message, confirmCh chan bool) {
			if err := c.sub(k, req, confirmCh); err != nil {
				logger.Info("restart re sub failed", k, err)
				c.SetSub(k, false)
			}
		}(k, req, confirmCh)
	}
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
		case <-time.After(c.options.ReceiveBufTimeout):
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

// ReceiveBuf - получение буфера входящих сообщений
func (c *ClientPublicConn) ReceiveBuf() chan *ClientPublicMessage {
	return c.receiveBuf
}

func (c *ClientPublicConn) setNeedRestart(value bool) {
	c.needRestartLock.Lock()
	c.needRestart = value
	c.needRestartLock.Unlock()
}

func (c *ClientPublicConn) needRestartSafe() bool {
	c.needRestartLock.RLock()
	res := c.needRestart
	c.needRestartLock.RUnlock()
	return res
}

// Close - закрытие соединения
func (c *ClientPublicConn) Close() {
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.wg.Wait()
		c.close()
	}
}

func (c *ClientPublicConn) close() {
	c.setNeedRestart(false)
	c.subDataLock.Lock()
	for _, v := range c.subData {
		close(v.confirm)
		v.confirm = nil
	}
	c.subDataLock.Unlock()
	if c.receiveBuf != nil {
		close(c.receiveBuf)
	}
	c.Conn.close()
}

// ClientPrivateMessage - сообщения, полученные через клиентское авторизованное соединение
type ClientPrivateMessage struct {
	Data []byte
	Conn *ClientPrivateConn
}

// ClientPrivateConn - клиентское авторизованное соединение с сервером.
//
// Использовать для клиентских соединений.
//
// Процесс восстановления соединения при отключении будет происходить автоматически, если установлены соответствующие
// опции. Подписки так же будут восстановлены.
type ClientPrivateConn struct {
	*ClientPublicConn
	options             *opts.ClientPrivateConnOptions
	authData            proto.Message
	authSuccessChan     chan bool
	authSuccessChanLock *sync.RWMutex
	receiveBuf          chan *ClientPrivateMessage
}

// NewClientPrivateConnWithRequest - установка клиентского авторизованного соединения с авторизацией при помощи сообщения-запроса
func NewClientPrivateConnWithRequest(url string, data proto.Message, options *opts.ClientPrivateConnOptions) (*ClientPrivateConn, error) {
	if err := opts.FillClientPrivateConnOptions(options); err != nil {
		return nil, err
	}
	header := http.Header{}
	header.Set(consts.HeaderContentType, options.ContentType)
	if options.FillVersion != nil {
		options.FillVersion(header)
	}
	c, err := newClientConn(url, header, options.ClientPublicConnOptions, false)
	if err != nil {
		return nil, err
	}
	res := &ClientPrivateConn{
		options:             options,
		ClientPublicConn:    c,
		authData:            data,
		authSuccessChan:     make(chan bool),
		authSuccessChanLock: &sync.RWMutex{},
		receiveBuf:          make(chan *ClientPrivateMessage, options.ReceiveBufSize),
	}
	res.start()
	return res, nil
}

// NewClientPrivateConnWithBasic - установка клиентского авторизованного соединения с авторизацией при помощи basic auth
func NewClientPrivateConnWithBasic(url, login, pass string, options *opts.ClientPrivateConnOptions) (*ClientPrivateConn, error) {
	if err := opts.FillClientPrivateConnOptions(options); err != nil {
		return nil, err
	}
	header := http.Header{}
	header.Set(consts.HeaderContentType, options.ContentType)
	header.Set(consts.AuthHeader,
		fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", login, pass)))))
	if options.FillVersion != nil {
		options.FillVersion(header)
	}
	c, err := newClientConn(url, header, options.ClientPublicConnOptions, false)
	if err != nil {
		return nil, err
	}
	res := &ClientPrivateConn{
		options:             options,
		ClientPublicConn:    c,
		authSuccessChan:     make(chan bool),
		authSuccessChanLock: &sync.RWMutex{},
		receiveBuf:          make(chan *ClientPrivateMessage, options.ReceiveBufSize),
	}
	res.start()
	return res, nil
}

// NewClientPrivateConnWithToken - установка клиентского авторизованного соединения с авторизацией при помощи jwt-авторизации
func NewClientPrivateConnWithToken(url, token string, options *opts.ClientPrivateConnOptions) (*ClientPrivateConn, error) {
	if err := opts.FillClientPrivateConnOptions(options); err != nil {
		return nil, err
	}
	header := http.Header{}
	header.Set(consts.HeaderContentType, options.ContentType)
	header.Set(consts.AuthHeader, fmt.Sprintf("Bearer %s", token))
	if options.FillVersion != nil {
		options.FillVersion(header)
	}
	c, err := newClientConn(url, header, options.ClientPublicConnOptions, false)
	if err != nil {
		return nil, err
	}
	res := &ClientPrivateConn{
		options:             options,
		ClientPublicConn:    c,
		authSuccessChan:     make(chan bool),
		authSuccessChanLock: &sync.RWMutex{},
		receiveBuf:          make(chan *ClientPrivateMessage, options.ReceiveBufSize),
	}
	res.start()
	return res, nil
}

func (c *ClientPrivateConn) start() {
	go c.pingPong()
	go c.listenReceive()
	go c.listenSend()
}

// Auth - функция отправки запроса авторизации. Нужна при восстановлении соединения и использовании авторизации с помощью сообщения
func (c *ClientPrivateConn) Auth() error {
	if c.authData != nil {
		c.SendData(&SentMessage{
			Data: c.authData,
		})
	}
	select {
	case _, _ = <-c.authSuccessChan:
		return nil
	case <-time.After(c.options.AuthTimeout):
		return AuthTimeoutErr
	}
}

// AuthConfirm - подтверждение успешной авторизации (вызывается обработчиком очереди сообщений,
// то есть ответственность за вызов лежит на пользователе SDK)
func (c *ClientPrivateConn) AuthConfirm() error {
	c.authSuccessChanLock.RLock()
	c.authSuccessChan <- true
	c.authSuccessChanLock.RUnlock()
	return nil
}

func (c *ClientPrivateConn) restart() {
	for {
		if !c.needRestartSafe() {
			break
		}
		dialer := &websocket.Dialer{}
		conn, _, err := dialer.Dial(c.url, c.header)
		if err != nil {
			logger.Info("restart dial connection failed", err)
			time.Sleep(c.options.RetryPeriod)
			continue
		}
		c.conn = conn
		c.start()
		if err := c.Auth(); err != nil {
			logger.Info("restart auth failed", err)
			if err := c.conn.Close(); err != nil {
				logger.Info("restart close after auth failed err", err)
			}
			break // listenReceive сделает еще один вызов рестарта
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
		case <-time.After(c.options.ReceiveBufTimeout):
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

// ReceiveBuf - получение буфера входящих сообщений
func (c *ClientPrivateConn) ReceiveBuf() chan *ClientPrivateMessage {
	return c.receiveBuf
}

// Close - закрытие соединения
func (c *ClientPrivateConn) Close() {
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.wg.Wait()
		c.close()
	}
}

func (c *ClientPrivateConn) close() {
	c.setNeedRestart(false)
	c.authSuccessChanLock.Lock()
	close(c.receiveBuf)
	close(c.authSuccessChan)
	c.authSuccessChan = nil
	c.authSuccessChanLock.Unlock()
	c.ClientPublicConn.Close()
}
