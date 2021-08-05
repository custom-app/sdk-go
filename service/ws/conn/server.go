package conn

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/loyal-inform/sdk-go/auth"
	"github.com/loyal-inform/sdk-go/auth/basic"
	"github.com/loyal-inform/sdk-go/auth/jwt/multiple"
	"github.com/loyal-inform/sdk-go/auth/jwt/single"
	"github.com/loyal-inform/sdk-go/logger"
	http2 "github.com/loyal-inform/sdk-go/service/http"
	"github.com/loyal-inform/sdk-go/structs"
	"github.com/loyal-inform/sdk-go/util/consts"
	"google.golang.org/protobuf/proto"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type AuthOptions struct {
	BasicAllowed, TokenAllowed, RequestAllowed bool
	MultipleTokens                             bool
	VersionHeader                              string
	VersionChecker                             http2.VersionChecker
	Disabled                                   []structs.Role
	ErrorMapper                                http2.AuthErrorMapper
	Timeout                                    time.Duration
}

type ServerPublicConnOptions struct {
	*Options
	Onclose func(int64)
}

type PublicMessage struct {
	Conn *ServerPublicConn
	Data []byte
}

// ServerPublicConn - соединение с клиентом на стороне сервера
type ServerPublicConn struct {
	*Conn
	opts       *ServerPublicConnOptions
	connId     int64
	receiveBuf chan *PublicMessage
}

func fillServerPublicOptions(opts *ServerPublicConnOptions) error {
	if opts.Options == nil {
		return OptsRequiredErr
	}
	return fillOpts(opts.Options)
}

func newServerPublicConn(conn *websocket.Conn, opts *ServerPublicConnOptions, needStart bool) (*ServerPublicConn, error) {
	if opts == nil {
		return nil, OptsRequiredErr
	}
	if err := fillServerPublicOptions(opts); err != nil {
		return nil, err
	}
	c, err := newConn(conn, opts.Options, false)
	if err != nil {
		return nil, err
	}
	res := &ServerPublicConn{
		Conn: c,
		opts: opts,
	}
	if needStart {
		res.receiveBuf = make(chan *PublicMessage, opts.ReceiveBufSize)
		res.start()
	}
	return res, nil
}

func NewServerPublicConn(conn *websocket.Conn, opts *ServerPublicConnOptions) (*ServerPublicConn, error) {
	return newServerPublicConn(conn, opts, true)
}

func UpgradePublicServerConn(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request,
	opts *ServerPublicConnOptions) (*ServerPublicConn, error) {
	defer r.Body.Close()
	if opts == nil {
		return nil, OptsRequiredErr
	}
	if err := fillServerPublicOptions(opts); err != nil {
		return nil, err
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	if err := r.Body.Close(); err != nil {
		return nil, err
	}
	res, err := NewServerPublicConn(conn, opts)
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
		if len(c.receiveBuf) == cap(c.receiveBuf) {
			logger.Log("receive buffer overflow")
			c.sendOverflowMessage()
			continue
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
	c.Close()
}

func (c *ServerPublicConn) ReceiveBuf() chan *PublicMessage {
	return c.receiveBuf
}

func (c *ServerPublicConn) SetConnId(value int64) {
	c.connId = value
}

func (c *ServerPublicConn) ConnId() int64 {
	return c.connId
}

func (c *ServerPublicConn) Close() {
	c.Conn.Close()
	if c.receiveBuf != nil {
		close(c.receiveBuf)
	}
	if c.opts.Onclose != nil {
		c.opts.Onclose(c.connId)
	}
}

type ServerPrivateConnOptions struct {
	*ServerPublicConnOptions
	AuthOptions *AuthOptions
	Onclose     func(*structs.Account, int64)
	Onauth      func(c *ServerPrivateConn)
}

type AuthRes struct {
	Resp    proto.Message
	Account *structs.Account
}

type PrivateMessage struct {
	Conn *ServerPrivateConn
	Data []byte
}

// ServerPrivateConn - авторизованное соединение с клиентом на стороне сервера
type ServerPrivateConn struct {
	*ServerPublicConn
	opts       *ServerPrivateConnOptions
	account    *structs.Account
	accLock    *sync.RWMutex
	authChLock *sync.Mutex
	authCh     chan *AuthRes
	receiveBuf chan *PrivateMessage
}

func fillServerPrivateOptions(opts *ServerPrivateConnOptions) error {
	if opts.ServerPublicConnOptions == nil {
		return OptsRequiredErr
	}
	return fillServerPublicOptions(opts.ServerPublicConnOptions)
}

func NewServerPrivateConn(conn *websocket.Conn, opts *ServerPrivateConnOptions) (*ServerPrivateConn, error) {
	if opts == nil {
		return nil, OptsRequiredErr
	}
	if err := fillServerPrivateOptions(opts); err != nil {
		return nil, err
	}
	c, err := newServerPublicConn(conn, opts.ServerPublicConnOptions, false)
	if err != nil {
		return nil, err
	}
	res := &ServerPrivateConn{
		opts:             opts,
		ServerPublicConn: c,
		accLock:          &sync.RWMutex{},
		authCh:           make(chan *AuthRes),
		authChLock:       &sync.Mutex{},
		receiveBuf:       make(chan *PrivateMessage, opts.ReceiveBufSize),
	}
	go res.start()
	return res, nil
}

func privateServerConnViaToken(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request,
	opts *ServerPrivateConnOptions, platform structs.Platform, versions []string, a string) (*ServerPrivateConn, error) {
	code, e := opts.AuthOptions.VersionChecker(platform, versions)
	if e != nil {
		http2.SendResponseWithContentType(w, r, code, e)
		return nil, fmt.Errorf("version check failed")
	}
	var (
		acc  *structs.Account
		resp proto.Message
		err  error
	)
	if opts.AuthOptions.MultipleTokens {
		acc, _, resp, err = multiple.AuthWithInfo(r.Context(), a[consts.TokenStartInd:], structs.PurposeAccess,
			platform, versions, opts.AuthOptions.Disabled...)
	} else {
		acc, resp, err = single.AuthWithInfo(r.Context(), a[consts.TokenStartInd:], structs.PurposeAccess,
			platform, versions, opts.AuthOptions.Disabled...)
	}
	if err != nil {
		code, e := opts.AuthOptions.ErrorMapper(err)
		http2.SendResponseWithContentType(w, r, code, e)
		return nil, err
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	if err := r.Body.Close(); err != nil {
		return nil, err
	}
	res, err := NewServerPrivateConn(conn, opts)
	if err != nil {
		return nil, err
	}
	res.SetAccount(acc)
	res.sendProto(resp)
	res.opts.Onauth(res)
	return res, nil
}

func privateServerConnViaBasic(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request,
	opts *ServerPrivateConnOptions, platform structs.Platform, versions []string, login, password string) (*ServerPrivateConn, error) {
	code, e := opts.AuthOptions.VersionChecker(platform, versions)
	if e != nil {
		http2.SendResponseWithContentType(w, r, code, e)
		return nil, fmt.Errorf("version check failed")
	}
	acc, resp, err := basic.AuthWithInfo(r.Context(), login, password, platform, versions, opts.AuthOptions.Disabled...)
	if err != nil {
		code, e := opts.AuthOptions.ErrorMapper(err)
		http2.SendResponseWithContentType(w, r, code, e)
		return nil, err
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	if err := r.Body.Close(); err != nil {
		return nil, err
	}
	res, err := NewServerPrivateConn(conn, opts)
	if err != nil {
		return nil, err
	}
	res.SetAccount(acc)
	res.sendProto(resp)
	res.opts.Onauth(res)
	return res, nil
}

func privateServerConnViaRequest(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request,
	opts *ServerPrivateConnOptions) (*ServerPrivateConn, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	if err := r.Body.Close(); err != nil {
		return nil, err
	}
	res, err := NewServerPrivateConn(conn, opts)
	if err != nil {
		return nil, err
	}
	go func() {
		authRes, err := res.auth()
		if err != nil {
			_, e := opts.AuthOptions.ErrorMapper(err)
			res.sendProto(e)
			logger.Log("auth wait failed. closing connection")
			res.Close()
		}
		res.SetAccount(authRes.Account)
		res.sendProto(authRes.Resp)
		res.opts.Onauth(res)
	}()
	return res, nil
}

func UpgradePrivateServerConn(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request,
	opts *ServerPrivateConnOptions) (*ServerPrivateConn, error) {
	defer r.Body.Close()
	if opts == nil {
		return nil, OptsRequiredErr
	}
	if err := fillServerPrivateOptions(opts); err != nil {
		return nil, err
	}
	a := r.Header.Get(consts.AuthHeader)
	isToken := strings.HasPrefix(a, consts.TokenStart)
	login, password, isBasic := r.BasicAuth()
	platform, versions := http2.ParseVersionHeader(r.Header, opts.AuthOptions.VersionHeader)
	var (
		res *ServerPrivateConn
		err error
	)
	if isToken && opts.AuthOptions.TokenAllowed {
		res, err = privateServerConnViaToken(upgrader, w, r, opts, platform, versions, a)
	} else if isBasic && opts.AuthOptions.BasicAllowed {
		res, err = privateServerConnViaBasic(upgrader, w, r, opts, platform, versions, login, password)
	} else if opts.AuthOptions.RequestAllowed {
		res, err = privateServerConnViaRequest(upgrader, w, r, opts)
	} else {
		code, e := opts.AuthOptions.ErrorMapper(auth.FailedAuthErr)
		http2.SendResponseWithContentType(w, r, code, e)
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
	case <-time.After(c.opts.AuthOptions.Timeout):
		return nil, AuthTimeoutErr
	}
}

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
		if len(c.receiveBuf) == cap(c.receiveBuf) {
			logger.Log("receive buffer overflow")
			c.sendOverflowMessage()
			continue
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
	c.Close()
}

func (c *ServerPrivateConn) ReceiveBuf() chan *PrivateMessage {
	return c.receiveBuf
}

func (c *ServerPrivateConn) ConnId() int64 {
	return c.connId
}

func (c *ServerPrivateConn) GetAccount() *structs.Account {
	c.accLock.RLock()
	res := c.account
	c.accLock.RUnlock()
	return res
}

func (c *ServerPrivateConn) SetAccount(acc *structs.Account) {
	c.accLock.Lock()
	c.account = acc
	c.accLock.Unlock()
}

func (c *ServerPrivateConn) Close() {
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.ServerPublicConn.Close()
		close(c.receiveBuf)
		if c.opts.Onclose != nil && c.account != nil {
			c.opts.Onclose(c.account, c.connId)
		}
	}
}
