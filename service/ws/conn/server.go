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
	*ConnOptions
	Onclose func(int64)
	ConnId  int64
}

// ServerPublicConn - соединение с клиентом на стороне сервера
type ServerPublicConn struct {
	*Conn
	opts *ServerPublicConnOptions
}

func fillServerPublicOptions(opts *ServerPublicConnOptions) error {
	if opts.ConnOptions == nil {
		return OptsRequiredErr
	}
	return fillOpts(opts.ConnOptions)
}

func NewServerPublicConn(conn *websocket.Conn, opts *ServerPublicConnOptions) (*ServerPublicConn, error) {
	if opts == nil {
		return nil, OptsRequiredErr
	}
	if err := fillServerPublicOptions(opts); err != nil {
		return nil, err
	}
	c, err := NewConn(conn, opts.ConnOptions)
	if err != nil {
		return nil, err
	}
	return &ServerPublicConn{
		Conn: c,
		opts: opts,
	}, nil
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
	return NewServerPublicConn(conn, opts)
}

func (c *ServerPublicConn) ConnId() int64 {
	return c.opts.ConnId
}

func (c *ServerPublicConn) Close() {
	c.Conn.Close()
	if c.opts.Onclose != nil {
		c.opts.Onclose(c.opts.ConnId)
	}
}

type ServerPrivateConnOptions struct {
	*ServerPublicConnOptions
	AuthOptions *AuthOptions
	Onclose     func(*structs.Account, int64)
}

type AuthRes struct {
	Resp    proto.Message
	Account *structs.Account
}

// ServerPrivateConn - авторизованное соединение с клиентом на стороне сервера
type ServerPrivateConn struct {
	*ServerPublicConn
	opts       *ServerPrivateConnOptions
	account    *structs.Account
	accLock    *sync.RWMutex
	authChLock *sync.Mutex
	authCh     chan *AuthRes
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
	c, err := NewServerPublicConn(conn, opts.ServerPublicConnOptions)
	if err != nil {
		return nil, err
	}
	return &ServerPrivateConn{
		opts:             opts,
		ServerPublicConn: c,
		accLock:          &sync.RWMutex{},
		authCh:           make(chan *AuthRes),
		authChLock:       &sync.Mutex{},
	}, nil
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
	res.SendProto(resp)
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
	res.SendProto(resp)
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
	authRes, err := res.auth()
	if err != nil {
		logger.Log("auth wait failed. closing connection")
		res.Close()
		return nil, err
	}
	res.SetAccount(authRes.Account)
	res.SendProto(authRes.Resp)
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
	if isToken && opts.AuthOptions.TokenAllowed {
		return privateServerConnViaToken(upgrader, w, r, opts, platform, versions, a)
	} else if isBasic && opts.AuthOptions.BasicAllowed {
		return privateServerConnViaBasic(upgrader, w, r, opts, platform, versions, login, password)
	} else if opts.AuthOptions.RequestAllowed {
		return privateServerConnViaRequest(upgrader, w, r, opts)
	} else {
		code, e := opts.AuthOptions.ErrorMapper(auth.FailedAuthErr)
		http2.SendResponseWithContentType(w, r, code, e)
		return nil, auth.FailedAuthErr
	}
}

func (c *ServerPrivateConn) auth() (*AuthRes, error) {
	select {
	case res := <-c.authCh:
		c.authChLock.Lock()
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

func (c *ServerPrivateConn) ConnId() int64 {
	return c.opts.ConnId
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
	c.Conn.Close()
	if c.opts.Onclose != nil {
		c.opts.Onclose(c.account, c.opts.ConnId)
	}
}
