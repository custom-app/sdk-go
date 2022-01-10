// Package conn - пакет работы с WebSocket соединениями.
//
// Содержит несколько оберток над соединениями, чтобы не дублировать код.
//
// Далее под клиентскими соединениями будут пониматься соединения на стороне клиента, а под серверными - соединения на стороне сервера.
package conn

import (
	"github.com/gorilla/websocket"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/service/ws/opts"
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
	marshaller = &protojson.MarshalOptions{
		UseProtoNames:   true,
		UseEnumNumbers:  true,
		EmitUnpopulated: true,
	}
)

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

// Conn - базовый тип соединения
type Conn struct {
	conn        *websocket.Conn // собственно, соединение
	options     *opts.Options   // опции
	contentType string          // content type использованный для создания соединения
	isAlive     int32           // счетчик для определения, живо ли соединение

	closeLock                *sync.Mutex           // мутекс для обработки конкурентных попыток закрыть соединение
	sendLock                 *sync.Mutex           // мутекс для последовательной обработки отправки/пинга/закрытия
	wg                       *sync.WaitGroup       // processing messages count
	sendDataLock             *sync.Mutex           // мутекс для последовательной обработки отправки сообщений
	sendBuf                  chan *SentMessage     // канал сообщений для отправки
	receiveBuf               chan *ReceivedMessage // канал полученных сообщений
	sendCloseCh, pingCloseCh chan bool             // каналы для выхода рутин отправки сообщений и пинг-понга

	subLock       *sync.RWMutex            // блокировка подписок
	subscriptions map[structs.SubKind]bool // активированные подписки
}

func newConn(conn *websocket.Conn, options *opts.Options, needStart bool) (*Conn, error) {
	if options == nil {
		return nil, opts.RequiredOptsErr
	}
	err := opts.FillOpts(options)
	if err != nil {
		return nil, err
	}
	res := &Conn{
		contentType:   options.ContentType,
		conn:          conn,
		sendLock:      &sync.Mutex{},
		wg:            &sync.WaitGroup{},
		subLock:       &sync.RWMutex{},
		subscriptions: map[structs.SubKind]bool{},
		sendDataLock:  &sync.Mutex{},
		sendBuf:       make(chan *SentMessage, options.SendBufSize),
		sendCloseCh:   make(chan bool),
		pingCloseCh:   make(chan bool),
		options:       options,
	}
	if needStart {
		res.receiveBuf = make(chan *ReceivedMessage, options.ReceiveBufSize)
		res.start()
	}

	return res, nil
}

// NewConn - создание структуры соединения из уже установленного WS-соединения
func NewConn(conn *websocket.Conn, options *opts.Options) (*Conn, error) {
	return newConn(conn, options, true)
}

// UpgradeConn - апгрейд соединения с помощью апгрейд запроса
func UpgradeConn(upgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request, options *opts.Options) (*Conn, error) {
	defer r.Body.Close()
	if options == nil {
		return nil, opts.RequiredOptsErr
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	res, err := NewConn(conn, options)
	if err != nil {
		return nil, err
	}
	if contentType := r.Header.Get(consts.HeaderContentType); contentType != "" {
		res.contentType = contentType
	}
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
	if c.options.ContentType == consts.JsonContentType {
		bytes, err = marshaller.Marshal(data)
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
	if c.options.ContentType == consts.JsonContentType {
		c.send(c.options.OverflowMsgJson)
	} else {
		c.send(c.options.OverflowMsgProto)
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
		c.wg.Add(1)
		select {
		case c.receiveBuf <- &ReceivedMessage{
			Conn: c,
			Data: msg,
		}:
			break
		case <-time.After(c.options.ReceiveBufTimeout):
			c.wg.Done()
			c.sendOverflowMessage()
		}
	}
}

func (c *Conn) listenReceiveWithStop() {
	c.listenReceive()
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.close()
	}
}

func (c *Conn) listenSend() {
L:
	for {
		select {
		case data := <-c.sendBuf:
			if data != nil {
				if data.Data != nil {
					c.sendProto(data.Data)
				}
				if data.IsResponse {
					c.wg.Done()
				}
			}
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
		if c.options.ContentType == consts.JsonContentType {
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
		case <-time.After(c.options.PingPeriod):
			c.processSendData(sendData{
				isPing: true,
			})
			break
		case <-c.pingCloseCh:
			break L
		}
	}
}

// ReceiveBuf - получение буфера входящих сообщений
func (c *Conn) ReceiveBuf() chan *ReceivedMessage {
	return c.receiveBuf
}

// SendMessage - отправка сообщения
func (c *Conn) SendMessage(msg proto.Message) {
	c.SendData(&SentMessage{
		Data: msg,
	})
}

// SendData - функция для упорядочивания отправки сообщений
func (c *Conn) SendData(msg *SentMessage) {
	c.sendDataLock.Lock()
	if c.IsAlive() {
		c.sendBuf <- msg
	}
	c.sendDataLock.Unlock()
}

// IsAlive - проверка состояния соединения
func (c *Conn) IsAlive() bool {
	return atomic.LoadInt32(&c.isAlive) == 0
}

// GetSub - есть ли подписка на топик kind
func (c *Conn) GetSub(kind structs.SubKind) bool {
	c.subLock.RLock()
	res := c.subscriptions[kind]
	c.subLock.RUnlock()
	return res
}

// SetSub - есть ли подписка на топик kind
func (c *Conn) SetSub(kind structs.SubKind, value bool) {
	c.subLock.Lock()
	c.subscriptions[kind] = value
	c.subLock.Unlock()
}

// ContentType - получение Content-Type, с которым было установлено соединение
func (c *Conn) ContentType() string {
	return c.contentType
}

// Close - закрытие соединения
func (c *Conn) Close() {
	if atomic.CompareAndSwapInt32(&c.isAlive, 0, 1) {
		c.wg.Wait()
		c.close()
	}
}

func (c *Conn) close() {
	c.sendDataLock.Lock()
	atomic.StoreInt32(&c.isAlive, 1)
	c.pingCloseCh <- true
	close(c.pingCloseCh)
	c.sendCloseCh <- true
	close(c.sendCloseCh)
	if err := c.conn.Close(); err != nil {
		logger.Log("ws close conn err: ", err)
	} else {
		logger.Log("close success")
	}
	if c.receiveBuf != nil {
		close(c.receiveBuf)
	}
	close(c.sendBuf)
	c.conn = nil
	c.sendDataLock.Unlock()
}
