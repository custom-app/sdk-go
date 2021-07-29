package ws

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
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	allCodes = []int{websocket.CloseNormalClosure, websocket.CloseGoingAway,
		websocket.CloseProtocolError, websocket.CloseUnsupportedData, websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure, websocket.CloseInvalidFramePayloadData, websocket.ClosePolicyViolation,
		websocket.CloseMessageTooBig, websocket.CloseMandatoryExtension, websocket.CloseInternalServerErr,
		websocket.CloseServiceRestart, websocket.CloseTryAgainLater, websocket.CloseTLSHandshake,
	}
)

type MessageHandler func(*Conn, *structs.Account, []byte) structs.Result

type closeHandler func(*structs.Account, *Conn)

type Subscriber func(*Conn, structs.SubKind) error

type RequestAuthFunc func(*Conn, []byte) (*structs.Account, proto.Message)

type AuthOptions struct {
	BasicAllowed, TokenAllowed, RequestAllowed bool
	MultipleTokens                             bool
	VersionHeader                              string
	VersionChecker                             http2.VersionChecker
	Disabled                                   []structs.Role
	RequestAuthFunc                            RequestAuthFunc
	ErrorMapper                                http2.AuthErrorMapper
	Timeout                                    time.Duration
}

type sendData struct {
	isShutdown, isPing bool
	data               []byte
}

type Conn struct {
	url, login, password string                   // информация для возможности переподключения. по url проверяется, можно ли делать переподключение
	conn                 *websocket.Conn          // собственно, соединение
	handler              MessageHandler           // обработчик всех входящих сообщений
	onclose              closeHandler             // коллбек закрытия соединения. используется для удаления соединения из пула
	isAlive              bool                     // работает ли соединение
	account              *structs.Account         // аккаунт соединения
	subscriptions        map[structs.SubKind]bool // активированные подписки
	subscriber           Subscriber               // функция реалиазация подписки
	subLock, accLock     *sync.RWMutex            // мутекс защиты подписок и привязанного к аккаунту коннекта
	sendLock             *sync.Mutex              // мутекс для последовательной обработки отправки/пинга/закрытия
	connId               int                      // индекс коннекта. уникальны в пространстве имен (роль,id)
	pool                 *Pool                    // ссылка на пул коннектов
	wg                   *sync.WaitGroup          // processing messages count
	contentType          string                   // content-type
}

func NewConn(w http.ResponseWriter, r *http.Request, handler MessageHandler, onclose closeHandler,
	authOptions *AuthOptions) (*Conn, error) {
	defer r.Body.Close()
	res := &Conn{
		handler:       handler,
		isAlive:       true,
		subscriptions: map[structs.SubKind]bool{},
		subLock:       &sync.RWMutex{},
		accLock:       &sync.RWMutex{},
		sendLock:      &sync.Mutex{},
		wg:            &sync.WaitGroup{},
		contentType:   r.Header.Get(consts.HeaderContentType),
	}
	a := r.Header.Get(consts.AuthHeader)
	isToken := strings.HasPrefix(a, consts.TokenStart)
	login, password, isBasic := r.BasicAuth()
	platform, versions := http2.ParseVersionHeader(r.Header, authOptions.VersionHeader)
	u := &websocket.Upgrader{}
	u.CheckOrigin = func(r *http.Request) bool {
		return true
	}
	if isToken && authOptions.TokenAllowed {
		code, e := authOptions.VersionChecker(platform, versions)
		if e != nil {
			http2.SendResponseWithContentType(w, r, code, e)
			return nil, fmt.Errorf("version check failed")
		}
		var (
			acc  *structs.Account
			resp proto.Message
			err  error
		)
		if authOptions.MultipleTokens {
			acc, _, resp, err = multiple.AuthWithInfo(r.Context(), a[consts.TokenStartInd:], structs.PurposeAccess,
				platform, versions, authOptions.Disabled...)
		} else {
			acc, resp, err = single.AuthWithInfo(r.Context(), a[consts.TokenStartInd:], structs.PurposeAccess,
				platform, versions, authOptions.Disabled...)
		}
		if err != nil {
			code, e := authOptions.ErrorMapper(err)
			http2.SendResponseWithContentType(w, r, code, e)
			return nil, err
		}
		conn, err := u.Upgrade(w, r, nil)
		if err != nil {
			return nil, err
		}
		if err := r.Body.Close(); err != nil {
			return nil, err
		}
		res.conn = conn
		res.account = acc
		go res.listenReceive()
		go res.pingPong()
		res.SendProto(resp)
	} else if isBasic && authOptions.BasicAllowed {
		code, e := authOptions.VersionChecker(platform, versions)
		if e != nil {
			http2.SendResponseWithContentType(w, r, code, e)
			return nil, fmt.Errorf("version check failed")
		}
		acc, resp, err := basic.AuthWithInfo(r.Context(), login, password, platform, versions, authOptions.Disabled...)
		if err != nil {
			code, e := authOptions.ErrorMapper(err)
			http2.SendResponseWithContentType(w, r, code, e)
			return nil, err
		}
		conn, err := u.Upgrade(w, r, nil)
		if err != nil {
			return nil, err
		}
		if err := r.Body.Close(); err != nil {
			return nil, err
		}
		res.conn = conn
		res.account = acc
		go res.listenReceive()
		go res.pingPong()
		res.SendProto(resp)
	} else if authOptions.RequestAllowed {
		conn, err := u.Upgrade(w, r, nil)
		if err != nil {
			return nil, err
		}
		if err := r.Body.Close(); err != nil {
			return nil, err
		}
		res.conn = conn
		if err := res.AuthWait(authOptions); err != nil {
			logger.Log("auth wait failed. closing connection")
			res.Close(false, false)
			return nil, err
		}
	} else {
		code, e := authOptions.ErrorMapper(auth.FailedAuthErr)
		http2.SendResponseWithContentType(w, r, code, e)
		return nil, auth.FailedAuthErr
	}
	res.onclose = onclose
	return res, nil
}

func NewClientConn(url, login, password string, handler MessageHandler, subscriber Subscriber) (*Conn, error) {
	res := &Conn{
		url:           url,
		login:         login,
		password:      password,
		handler:       handler,
		subscriptions: map[structs.SubKind]bool{},
		subLock:       &sync.RWMutex{},
		accLock:       &sync.RWMutex{},
		sendLock:      &sync.Mutex{},
		wg:            &sync.WaitGroup{},
	}

	return res, nil
}

func (c *Conn) Start() {
	dialer := &websocket.Dialer{}
	conn, _, err := dialer.Dial(c.url, nil)
	if err != nil {
		logger.Log("setup connection err: ", err)
		c.restartUnsafe()
	} else {
		logger.Log("connection successful")
		c.conn = conn
		c.isAlive = true
		if err := c.SelfAuth(c.login, c.password); err != nil {
			logger.Log("auth in connection err: ", err)
			c.restartUnsafe()
		}
	}
	logger.Log("connection established")
}

var marshaler = &protojson.MarshalOptions{
	UseProtoNames:   true,
	UseEnumNumbers:  true,
	EmitUnpopulated: true,
}

func (c *Conn) SendProto(data proto.Message) {
	var (
		bytes []byte
		err   error
	)
	if c.contentType == "application/json" {
		bytes, err = marshaler.Marshal(data)
	} else {
		bytes, err = proto.Marshal(data)
	}
	if err != nil {
		logger.Log("ws marshal proto err: ", err)
		return
	}
	c.Send(bytes)
}

func (c *Conn) processSendData(data sendData) {
	c.sendLock.Lock()
	if c.IsAlive() {
		if data.isShutdown {
			c.handler = nil // to stop handle messages
			c.wg.Wait()     // wait until finish all started messages
			c.Close(false, false)
		} else if data.isPing {
			c.ping()
		} else {
			c.writeBytes(data.data)
		}
	}
	c.sendLock.Unlock()
}

func (c *Conn) Send(data []byte) {
	c.processSendData(sendData{
		data: data,
	})
}

func (c *Conn) Shutdown() {
	c.processSendData(sendData{
		isShutdown: true,
	})
}

func (c *Conn) listenReceive() {
	c.isAlive = true
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
			c.Close(true, false)
			break
		}
		if c.handler == nil {
			continue
		}
		c.wg.Add(1)
		go func() {
			acc := c.GetAccount()
			res := c.handler(c, acc, msg)
			if res != nil {
				logger.Log("sending response: ", res)
				if res.GetResponse() != nil {
					c.SendProto(res.GetResponse())
				}
				if acc != nil {
					c.pool.HandleResult(res, &SenderInfo{
						Role:  acc.Role,
						Id:    acc.Id,
						Index: c.connId,
					})
				}
			}
			c.wg.Done()
		}()
	}
	c.restartUnsafe()
	if c.onclose != nil {
		c.onclose(c.GetAccount(), c)
	}
}

func (c *Conn) ping() {
	if err := c.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
		logger.Log("send ping err: ", err)
	}
}

func (c *Conn) writeBytes(data []byte) {
	if data != nil {
		if c.contentType == "application/json" {
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				logger.Log("ws send msg err: ", err)
			}
		} else {
			if err := c.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				logger.Log("ws send msg err: ", err)
			}
		}
	}
}

func (c *Conn) pingPong() {
	for {
		time.Sleep(time.Second * 45)
		if c.isAlive {
			c.processSendData(sendData{
				isPing: true,
			})
		} else {
			break
		}
	}
}

func (c *Conn) restart() error {
	if c.url != "" {
		dialer := &websocket.Dialer{}
		conn, _, err := dialer.Dial(c.url, nil)
		if err != nil {
			return err
		}
		logger.Log("connection successful restart", c.url)
		c.conn, c.isAlive = conn, true
		if err := c.SelfAuth(c.login, c.password); err != nil {
			return err
		}
		subs := c.subscriptions
		c.subscriptions = map[structs.SubKind]bool{}
		for k := range subs {
			if err := c.Sub(k); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Conn) Sub(kind structs.SubKind) error {
	return c.subscriber(c, kind)
}

func (c *Conn) restartUnsafe() {
	if c.url != "" {
		for {
			time.Sleep(250 * time.Millisecond)
			if err := c.restart(); err != nil {
				logger.Log("failed restart: ", c.url, err)
			} else {
				break
			}
		}
	}
}

func (c *Conn) IsAlive() bool {
	return c.isAlive
}

func (c *Conn) ContentType() string {
	return c.contentType
}

func (c *Conn) GetAccount() *structs.Account {
	c.accLock.RLock()
	res := c.account
	c.accLock.RUnlock()
	return res
}

func (c *Conn) SetAccount(acc *structs.Account) {
	c.accLock.Lock()
	c.account = acc
	c.accLock.Unlock()
}

func (c *Conn) ToggleSub(kind structs.SubKind) {
	c.subLock.Lock()
	c.subscriptions[kind] = !c.subscriptions[kind]
	c.subLock.Unlock()
}

func (c *Conn) Close(connFailed, force bool) {
	logger.Log("closing for", c.GetAccount(), connFailed, c.isAlive)
	if c.isAlive {
		if force {
			c.url = ""
		}
		c.isAlive = false
		if !connFailed {
			if err := c.conn.Close(); err != nil {
				logger.Log("ws close conn err: ", err)
			}
		}
	}
}
