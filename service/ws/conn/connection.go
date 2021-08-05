package conn

import (
	"errors"
	"github.com/gorilla/websocket"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/structs"
	"github.com/loyal-inform/sdk-go/util/consts"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var (
	allCodes = []int{websocket.CloseNormalClosure, websocket.CloseGoingAway,
		websocket.CloseProtocolError, websocket.CloseUnsupportedData, websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure, websocket.CloseInvalidFramePayloadData, websocket.ClosePolicyViolation,
		websocket.CloseMessageTooBig, websocket.CloseMandatoryExtension, websocket.CloseInternalServerErr,
		websocket.CloseServiceRestart, websocket.CloseTryAgainLater, websocket.CloseTLSHandshake,
	}
	marshaler = &protojson.MarshalOptions{
		UseProtoNames:   true,
		UseEnumNumbers:  true,
		EmitUnpopulated: true,
	}
	OptsRequiredErr = errors.New("opts required")
)

const (
	defaultReceiveBufSize = 10
	defaultSendBufSize    = 10
	defaultBufTimeout     = 10 * time.Second
	defaultPingPeriod     = time.Second * 45
)

type MessageHandler func(*Conn, *structs.Account, []byte) structs.Result

type Subscriber func(*Conn, structs.SubKind) error

type sendData struct {
	isPing bool
	data   []byte
}

type ReceivedMessage struct {
	Conn *Conn
	Data []byte
}

type SentMessage struct {
	Data       proto.Message
	IsResponse bool
}

type Options struct {
	ContentType                       string
	OverflowMsg                       proto.Message
	OverflowMsgJson, OverflowMsgProto []byte
	ReceiveBufSize                    int
	SendBufSize                       int
	ReceiveBufTimeout, PingPeriod     time.Duration
}

type Conn struct {
	conn        *websocket.Conn // собственно, соединение
	opts        *Options
	contentType string
	isAlive     int32 // счетчик для определения, живо ли соединение

	sendLock                 *sync.Mutex           // мутекс для последовательной обработки отправки/пинга/закрытия
	wg                       *sync.WaitGroup       // processing messages count
	sendBuf                  chan proto.Message     // канал сообщений для отправки
	receiveBuf               chan *ReceivedMessage // канал полученных сообщений
	sendCloseCh, pingCloseCh chan bool             // каналы для выхода рутин отправки сообщений и пинг-понга

	subLock       *sync.RWMutex            // блокировка подписок
	subscriptions map[structs.SubKind]bool // активированные подписки
}

func fillOpts(opts *Options) error {
	var err error
	if opts.OverflowMsgJson == nil {
		opts.OverflowMsgJson, err = marshaler.Marshal(opts.OverflowMsg)
		if err != nil {
			return err
		}
	}
	if opts.OverflowMsgProto == nil {
		opts.OverflowMsgProto, err = proto.Marshal(opts.OverflowMsg)
		if err != nil {
			return err
		}
	}
	if opts.ReceiveBufTimeout == 0 {
		opts.ReceiveBufTimeout = defaultBufTimeout
	}
	if opts.PingPeriod == 0 {
		opts.PingPeriod = defaultPingPeriod
	}
	if opts.ContentType == "" {
		opts.ContentType = consts.ProtoContentType
	}
	if opts.ReceiveBufSize == 0 {
		opts.ReceiveBufSize = defaultReceiveBufSize
	}
	if opts.SendBufSize == 0 {
		opts.SendBufSize = defaultSendBufSize
	}
	return nil
}

func newConn(conn *websocket.Conn, opts *Options, needStart bool) (*Conn, error) {
	if opts == nil {
		return nil, OptsRequiredErr
	}
	err := fillOpts(opts)
	if err != nil {
		return nil, err
	}
	res := &Conn{
		contentType: opts.ContentType,
		conn:        conn,
		sendLock:    &sync.Mutex{},
		wg:          &sync.WaitGroup{},
		sendBuf:     make(chan proto.Message, opts.SendBufSize),
		sendCloseCh: make(chan bool),
		pingCloseCh: make(chan bool),
		opts:        opts,
	}
	if needStart {
		res.receiveBuf = make(chan *ReceivedMessage, opts.ReceiveBufSize)
		res.start()
	}

	return res, nil
}

func NewConn(conn *websocket.Conn, opts *Options) (*Conn, error) {
	return newConn(conn, opts, true)
}

func UpgradeConn(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request, opts *Options) (*Conn, error) {
	defer r.Body.Close()
	if opts == nil {
		return nil, OptsRequiredErr
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	res, err := NewConn(conn, opts)
	if err != nil {
		return nil, err
	}
	res.contentType = r.Header.Get(consts.HeaderContentType)
	return res, nil
}

func (c *Conn) start() {
	go c.pingPong()
	go c.listenReceiveWithStop()
	go c.listenSend()
}

func (c *Conn) sendProto(data proto.Message) {
	var (
		bytes []byte
		err   error
	)
	if c.opts.ContentType == consts.JsonContentType {
		bytes, err = marshaler.Marshal(data)
	} else {
		bytes, err = proto.Marshal(data)
	}
	if err != nil {
		logger.Log("ws marshal proto err: ", err)
		return
	}
	c.send(bytes)
}

func (c *Conn) processSendData(data sendData) {
	c.sendLock.Lock()
	if data.isPing {
		c.ping()
	} else {
		c.writeBytes(data.data)
	}
	c.sendLock.Unlock()
}

func (c *Conn) send(data []byte) {
	c.processSendData(sendData{
		data: data,
	})
}

func (c *Conn) sendOverflowMessage() {
	if c.opts.ContentType == consts.JsonContentType {
		c.send(c.opts.OverflowMsgJson)
	} else {
		c.send(c.opts.OverflowMsgProto)
	}
}

func (c *Conn) listenReceive() {
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
		case c.receiveBuf <- &ReceivedMessage{
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

func (c *Conn) listenReceiveWithStop() {
	c.listenReceive()
	c.Close()
}

func (c *Conn) listenSend() {
L:
	for {
		select {
		case data := <-c.sendBuf:
			if data != nil {
				c.sendProto(data)
			}
			c.wg.Done()
			break
		case <-c.sendCloseCh:
			break L
		}
	}
}

func (c *Conn) ping() {
	if err := c.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
		logger.Log("send ping err: ", err)
	}
}

func (c *Conn) writeBytes(data []byte) {
	if data != nil {
		if c.opts.ContentType == consts.JsonContentType {
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
L:
	for {
		select {
		case <-time.After(c.opts.PingPeriod):
			c.processSendData(sendData{
				isPing: true,
			})
			break
		case <-c.pingCloseCh:
			break L
		}
	}
}

func (c *Conn) ReceiveBuf() chan *ReceivedMessage {
	return c.receiveBuf
}

func (c *Conn) SendBuf() chan proto.Message {
	return c.sendBuf
}

func (c *Conn) IsAlive() bool {
	return atomic.LoadInt32(&c.isAlive) == 0
}

func (c *Conn) GetSub(kind structs.SubKind) bool {
	c.subLock.RLock()
	res := c.subscriptions[kind]
	c.subLock.RUnlock()
	return res
}

func (c *Conn) SetSub(kind structs.SubKind, value bool) {
	c.subLock.Lock()
	c.subscriptions[kind] = value
	c.subLock.Unlock()
}

func (c *Conn) ContentType() string {
	return c.opts.ContentType
}

func (c *Conn) Close() {
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.wg.Wait()
		c.pingCloseCh <- true
		close(c.pingCloseCh)
		c.sendCloseCh <- true
		close(c.sendCloseCh)
		if err := c.conn.Close(); err != nil {
			logger.Log("ws close conn err: ", err)
		}
		if c.receiveBuf != nil {
			close(c.receiveBuf)
		}
		close(c.sendBuf)
		c.conn = nil
	}
}
