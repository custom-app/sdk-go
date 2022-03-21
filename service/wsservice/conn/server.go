package conn

import (
	"fmt"
	"github.com/custom-app/sdk-go/auth"
	"github.com/custom-app/sdk-go/auth/basic"
	"github.com/custom-app/sdk-go/auth/jwt"
	"github.com/custom-app/sdk-go/logger"
	"github.com/custom-app/sdk-go/service/httpservice"
	"github.com/custom-app/sdk-go/service/wsservice/opts"
	"github.com/custom-app/sdk-go/structs"
	"github.com/custom-app/sdk-go/util/consts"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// PublicMessage - сообщение из публичного соединения
type PublicMessage struct {
	Conn *ServerPublicConn
	Data []byte
}

// ServerPublicConn - соединение с клиентом на стороне сервера
type ServerPublicConn struct {
	*Conn
	opts       *opts.ServerPublicConnOptions
	connIdLock *sync.RWMutex
	connId     int64
	receiveBuf chan *PublicMessage
	reqHeader  http.Header
	Onclose    func(int64)
}

func newServerPublicConn(conn *websocket.Conn, options *opts.ServerPublicConnOptions,
	onclose func(int64), needStart bool, reqHeader http.Header) (*ServerPublicConn, error) {
	if options == nil {
		return nil, opts.RequiredOptsErr
	}
	if err := opts.FillServerPublicOptions(options); err != nil {
		return nil, err
	}
	c, err := newConn(conn, options.Options, false)
	if err != nil {
		return nil, err
	}
	res := &ServerPublicConn{
		Conn:       c,
		opts:       options,
		Onclose:    onclose,
		connIdLock: &sync.RWMutex{},
		reqHeader:  reqHeader,
	}
	if needStart {
		res.receiveBuf = make(chan *PublicMessage, options.ReceiveBufSize)
		res.start()
	}
	return res, nil
}

// NewServerPublicConn - создание публичного серверного соединения
func NewServerPublicConn(conn *websocket.Conn, options *opts.ServerPublicConnOptions,
	onclose func(int64), reqHeader http.Header) (*ServerPublicConn, error) {
	return newServerPublicConn(conn, options, onclose, true, reqHeader)
}

// UpgradePublicServerConn - апгрейд публичного серверного соединения с помощью апгрейд запроса
func UpgradePublicServerConn(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request,
	options *opts.ServerPublicConnOptions, onclose func(int64)) (*ServerPublicConn, error) {
	defer r.Body.Close()
	if options == nil {
		return nil, opts.RequiredOptsErr
	}
	if err := opts.FillServerPublicOptions(options); err != nil {
		return nil, err
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	if err := r.Body.Close(); err != nil {
		return nil, err
	}
	res, err := NewServerPublicConn(conn, options, onclose, r.Header)
	if err != nil {
		return nil, err
	}
	res.contentType = r.Header.Get(consts.HeaderContentType)
	return res, nil
}

func (c *ServerPublicConn) start() {
	go c.pingPong()
	go c.listenReceiveWithStop()
	go c.listenSend()
}

func (c *ServerPublicConn) listenReceive() {
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
		c.wg.Add(1)
		select {
		case c.receiveBuf <- &PublicMessage{
			Conn: c,
			Data: msg,
		}:
			break
		case <-time.After(c.opts.ReceiveBufTimeout):
			c.wg.Done()
			c.sendOverflowMessage()
		}
	}
}

func (c *ServerPublicConn) listenReceiveWithStop() {
	c.listenReceive()
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.close()
	}
}

// ReceiveBuf - получение буфера входящих сообщений
func (c *ServerPublicConn) ReceiveBuf() chan *PublicMessage {
	return c.receiveBuf
}

// SetConnId - изменение id соединения
func (c *ServerPublicConn) SetConnId(value int64) {
	c.connIdLock.Lock()
	c.connId = value
	c.connIdLock.Unlock()
}

// HeaderValue - получение значения заголовка запроса апгрейда соединения
func (c *ServerPublicConn) HeaderValue(key string) string {
	if c.reqHeader == nil {
		return ""
	}
	return c.reqHeader.Get(key)
}

// ConnId - получение id соединения
func (c *ServerPublicConn) ConnId() int64 {
	c.connIdLock.RLock()
	res := c.connId
	c.connIdLock.RUnlock()
	return res
}

// Close - закрытие соединения
func (c *ServerPublicConn) Close() {
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.wg.Wait()
		c.close()
	}
}

func (c *ServerPublicConn) close() {
	c.Conn.close()
	if c.receiveBuf != nil {
		close(c.receiveBuf)
	}
	if c.Onclose != nil {
		c.Onclose(c.ConnId())
	}
}

// AuthRes - результат авторизации
type AuthRes struct {
	Resp    proto.Message
	Account *structs.Account
}

// PrivateMessage - сообщение из авторизованного серверного соединения
type PrivateMessage struct {
	Conn *ServerPrivateConn
	Data []byte
}

// ServerPrivateConn - авторизованное серверное соединение с клиентом
type ServerPrivateConn struct {
	*ServerPublicConn
	options    *opts.ServerPrivateConnOptions
	account    *structs.Account
	accLock    *sync.RWMutex
	authChLock *sync.Mutex
	authCh     chan *AuthRes
	receiveBuf chan *PrivateMessage
	Onclose    func(*structs.Account, int64)
	onauth     func(*ServerPrivateConn)
}

// NewServerPrivateConn - создание структуры авторизованного серверного соединения с имеющимся установленным WS-соединением
func NewServerPrivateConn(conn *websocket.Conn, options *opts.ServerPrivateConnOptions,
	onauth func(*ServerPrivateConn), onclose func(*structs.Account, int64),
	reqHeader http.Header) (*ServerPrivateConn, error) {
	if options == nil {
		return nil, opts.RequiredOptsErr
	}
	if err := opts.FillServerPrivateOptions(options); err != nil {
		return nil, err
	}
	c, err := newServerPublicConn(conn, options.ServerPublicConnOptions, nil, false, reqHeader)
	if err != nil {
		return nil, err
	}
	res := &ServerPrivateConn{
		options:          options,
		ServerPublicConn: c,
		accLock:          &sync.RWMutex{},
		authCh:           make(chan *AuthRes),
		authChLock:       &sync.Mutex{},
		receiveBuf:       make(chan *PrivateMessage, options.ReceiveBufSize),
		Onclose:          onclose,
		onauth:           onauth,
	}
	go res.start()
	return res, nil
}

func privateServerConnViaToken(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request,
	options *opts.ServerPrivateConnOptions, platform structs.Platform, versions []string, a string,
	onauth func(*ServerPrivateConn), onclose func(*structs.Account, int64)) (*ServerPrivateConn, error) {
	code, e := options.AuthOptions.VersionChecker(platform, versions)
	if e != nil {
		httpservice.SendResponseWithContentType(w, r, code, e)
		return nil, fmt.Errorf("version check failed")
	}
	var (
		acc  *structs.Account
		resp proto.Message
		err  error
	)
	acc, _, resp, err = jwt.AuthWithInfo(r.Context(), a[consts.TokenStartInd:], jwt.PurposeAccess,
		platform, versions, options.AuthOptions.Disabled...)
	if err != nil {
		code, e := options.AuthOptions.ErrorMapper(err)
		httpservice.SendResponseWithContentType(w, r, code, e)
		return nil, err
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	if err := r.Body.Close(); err != nil {
		return nil, err
	}
	res, err := NewServerPrivateConn(conn, options, onauth, onclose, r.Header)
	if err != nil {
		return nil, err
	}
	res.SetAccount(acc)
	res.sendProto(resp)
	res.onauth(res)
	return res, nil
}

func privateServerConnViaBasic(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request,
	options *opts.ServerPrivateConnOptions, platform structs.Platform, versions []string, login, password string,
	onauth func(*ServerPrivateConn), onclose func(*structs.Account, int64)) (*ServerPrivateConn, error) {
	code, e := options.AuthOptions.VersionChecker(platform, versions)
	if e != nil {
		httpservice.SendResponseWithContentType(w, r, code, e)
		return nil, fmt.Errorf("version check failed")
	}
	acc, resp, err := basic.AuthWithInfo(r.Context(), login, password, platform, versions, options.AuthOptions.Disabled...)
	if err != nil {
		code, e := options.AuthOptions.ErrorMapper(err)
		httpservice.SendResponseWithContentType(w, r, code, e)
		return nil, err
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	if err := r.Body.Close(); err != nil {
		return nil, err
	}
	res, err := NewServerPrivateConn(conn, options, onauth, onclose, r.Header)
	if err != nil {
		return nil, err
	}
	res.SetAccount(acc)
	res.sendProto(resp)
	res.onauth(res)
	return res, nil
}

func privateServerConnViaRequest(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request,
	options *opts.ServerPrivateConnOptions, onauth func(*ServerPrivateConn),
	onclose func(*structs.Account, int64)) (*ServerPrivateConn, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	if err := r.Body.Close(); err != nil {
		return nil, err
	}
	res, err := NewServerPrivateConn(conn, options, onauth, onclose, r.Header)
	if err != nil {
		return nil, err
	}
	go func() {
		authRes, err := res.auth()
		if err != nil {
			_, e := options.AuthOptions.ErrorMapper(err)
			res.SendData(&SentMessage{
				Data:       e,
				IsResponse: err != AuthTimeoutErr,
			})
			if res.IsAlive() {
				logger.Log("auth wait failed. closing connection", r.UserAgent(),
					r.Header.Get(consts.HeaderXForwardedFor), e)
				res.Close()
			}
			return
		}
		res.SetAccount(authRes.Account)
		res.SendData(&SentMessage{
			Data:       authRes.Resp,
			IsResponse: true,
		})
		res.onauth(res)
	}()
	return res, nil
}

// UpgradePrivateServerConn - апгрейд авторизованного серверного соединения с помощью апгрейд запроса
func UpgradePrivateServerConn(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request,
	options *opts.ServerPrivateConnOptions, onauth func(*ServerPrivateConn),
	onclose func(*structs.Account, int64)) (*ServerPrivateConn, error) {
	defer r.Body.Close()
	if options == nil {
		return nil, opts.RequiredOptsErr
	}
	if err := opts.FillServerPrivateOptions(options); err != nil {
		return nil, err
	}
	a := r.Header.Get(consts.AuthHeader)
	isToken := strings.HasPrefix(a, consts.TokenStart)
	login, password, isBasic := r.BasicAuth()
	platform, versions := httpservice.ParseVersionHeader(r.Header, options.AuthOptions.VersionHeader)
	var (
		res *ServerPrivateConn
		err error
	)
	if isToken && options.AuthOptions.TokenAllowed {
		res, err = privateServerConnViaToken(upgrader, w, r, options, platform, versions, a, onauth, onclose)
	} else if isBasic && options.AuthOptions.BasicAllowed {
		res, err = privateServerConnViaBasic(upgrader, w, r, options, platform, versions, login, password, onauth, onclose)
	} else if options.AuthOptions.RequestAllowed {
		res, err = privateServerConnViaRequest(upgrader, w, r, options, onauth, onclose)
	} else {
		code, e := options.AuthOptions.ErrorMapper(auth.FailedAuthErr)
		httpservice.SendResponseWithContentType(w, r, code, e)
		res, err = nil, auth.FailedAuthErr
	}
	if err != nil {
		return nil, err
	}
	res.contentType = r.Header.Get(consts.HeaderContentType)
	return res, nil
}

func (c *ServerPrivateConn) start() {
	go c.pingPong()
	go c.listenReceiveWithStop()
	go c.listenSend()
}

func (c *ServerPrivateConn) auth() (*AuthRes, error) {
	select {
	case res := <-c.authCh:
		c.authChLock.Lock()
		close(c.authCh)
		c.authCh = nil
		c.authChLock.Unlock()
		return res, nil
	case <-time.After(c.options.AuthOptions.Timeout):
		return nil, AuthTimeoutErr
	}
}

// AuthConfirm - подтверждение успешной авторизации (вызывается обработчиком очереди сообщений,
// то есть ответственность за вызов лежит на пользователе SDK)
func (c *ServerPrivateConn) AuthConfirm(res *AuthRes) {
	c.authChLock.Lock()
	if c.authCh != nil {
		c.authCh <- res
	}
	c.authChLock.Unlock()
}

func (c *ServerPrivateConn) listenReceive() {
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, allCodes...) {
				logger.Log("server private normal close")
			} else if websocket.IsUnexpectedCloseError(err, allCodes...) {
				logger.Log("server private unexpected close")
			} else {
				logger.Log("server private ws read message err: ", err)
			}
			break
		}
		if !c.IsAlive() {
			break
		}
		c.wg.Add(1)
		select {
		case c.receiveBuf <- &PrivateMessage{
			Conn: c,
			Data: msg,
		}:
			break
		case <-time.After(c.opts.ReceiveBufTimeout):
			c.wg.Done()
			c.sendOverflowMessage()
		}
	}
}

func (c *ServerPrivateConn) listenReceiveWithStop() {
	c.listenReceive()
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.close()
	}
}

// ReceiveBuf - получение буфера входящих сообщений
func (c *ServerPrivateConn) ReceiveBuf() chan *PrivateMessage {
	return c.receiveBuf
}

// GetAccount - получение аккаунта соединения
func (c *ServerPrivateConn) GetAccount() *structs.Account {
	c.accLock.RLock()
	res := c.account
	c.accLock.RUnlock()
	return res
}

// SetAccount - привязка аккаунта к соединению
func (c *ServerPrivateConn) SetAccount(acc *structs.Account) {
	c.accLock.Lock()
	c.account = acc
	c.accLock.Unlock()
}

// Close - закрытие соединения
func (c *ServerPrivateConn) Close() {
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.wg.Wait()
		c.close()
	}
}

func (c *ServerPrivateConn) close() {
	c.ServerPublicConn.close()
	close(c.receiveBuf)
	if c.Onclose != nil && c.account != nil {
		c.Onclose(c.account, c.ConnId())
	}
}
