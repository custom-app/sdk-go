package apiclient

import (
	"errors"
	"fmt"
	"github.com/loyal-inform/sdk-go/service/workerpool/workerpoolws"
	"github.com/loyal-inform/sdk-go/service/wsservice/conn"
	"github.com/loyal-inform/sdk-go/service/wsservice/opts"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"sync"
	"sync/atomic"
)

var (
	AddressRequiredErr     = errors.New("address required")      // AddressRequiredErr - пустой адрес
	ConnIndexOutOfRangeErr = errors.New("conn ind out of range") // ConnIndexOutOfRangeErr - в клиенте нет соединения с таким индексом
	UnhandledMessageErr    = errors.New("unhandled message")     // UnhandledMessageErr - для какого-то из сообщений не нашлось обработчика
)

// WsMessageKind - вспомогательный тип для определения типа сообщения
type WsMessageKind int

const (
	MessageKindAuth     = WsMessageKind(iota) // MessageKindAuth - ответ на запрос авторизации
	MessageKindSub                            // MessageKindSub - сообщение по подписке
	MessageKindResponse                       // MessageKindResponse - ответ на произвольный запрос
)

// ConnectionInfo - данные соединения
type ConnectionInfo struct {
	Address     string                         // Address - адрес для подключения
	ConnOptions *opts.ClientPrivateConnOptions // ConnOptions - опции подключения
	SubHandler  WsSubHandler                   // SubHandler - обработчик любого сообщения по подписке
	conn        *conn.ClientPrivateConn
	worker      *workerpoolws.ClientPrivateWorker
	count       uint64
	messages    *sync.Map
	subKinds    *sync.Map
}

// WsMessageHandler - сигнатура функции обработчика ws-сообщения
type WsMessageHandler func(msg proto.Message) (needRetry bool, err error)

// WsSubHandler - сигнатура функции обработчика ws-сообщения по подписке.
// Отличие в том, что здесь нет такого понятия как retry
type WsSubHandler func(msg proto.Message) error

// WsMsgParser - сигнатура функции парсинга сообщения
type WsMsgParser func([]byte) (WsMessageKind, WsMessage, error)

type messageInfo struct {
	handler WsMessageHandler
	msg     WsMessage
	isSub   bool
}

// WsMessage - сообщения для отправки по WS, обязательна возможность получения/присваивания id запроса
//
// Если в прото сообщении сделать поле id, то функция GetId сгенерится сама, а вот функцию SetId надо добавить
// самостоятельно, это можно сделать с помощью объявления нового типа, в который будет встроен(embedding) тип запроса
type WsMessage interface {
	proto.Message
	GetId() uint64
	SetId(id uint64)
}

// WsClient - клиент для отправки сообщения по WebSocket
type WsClient struct {
	*client
	connections []*ConnectionInfo
	msgParser   WsMsgParser
}

func NewWsClient(connections []*ConnectionInfo, accessToken string, accessExpiresAt int64,
	refreshToken string, refreshExpiresAt int64,
	refresh refreshFunc, notifier errorNotifier, msgParser WsMsgParser) (*WsClient, error) {
	c, err := newClient(accessToken, accessExpiresAt, refreshToken, refreshExpiresAt, refresh, notifier)
	if err != nil {
		return nil, err
	}
	return &WsClient{
		client:      c,
		connections: connections,
		msgParser:   msgParser,
	}, nil
}

func (c *WsClient) Start() error {
	if err := c.client.start(); err != nil {
		return err
	}
	for _, connection := range c.connections {
		if connection.Address == "" {
			return AddressRequiredErr
		}
		var err error
		connection.messages, connection.subKinds = &sync.Map{}, &sync.Map{}
		connection.conn, err = conn.NewClientPrivateConnWithToken(connection.Address, c.getAccessToken(), connection.ConnOptions)
		if err != nil {
			return err
		}
		connection.worker = workerpoolws.NewClientPrivateWorker(connection.conn.ReceiveBuf(), c.handleMessage(connection))
		go connection.worker.Run()
		if err := connection.conn.Auth(); err != nil {
			return err
		}
	}
	return nil
}

func (c *WsClient) handleMessage(connection *ConnectionInfo) workerpoolws.ClientPrivateHandler {
	return func(msg *conn.ClientPrivateMessage) {
		kind, data, err := c.msgParser(msg.Data)
		if err != nil {
			c.notifier(err)
			return
		}
		switch kind {
		case MessageKindAuth:
			if err := connection.conn.AuthConfirm(); err != nil {
				c.notifier(err)
				return
			}
		case MessageKindSub:
			if connection.SubHandler != nil {
				if err := connection.SubHandler(data); err != nil {
					c.notifier(fmt.Errorf("sub handle failed: %w", err))
				}
			}
		case MessageKindResponse:
			value, ok := connection.messages.Load(data.GetId())
			if !ok {
				c.notifier(UnhandledMessageErr)
				return
			}
			msgInfo := value.(*messageInfo)
			if needRetry, err := msgInfo.handler(data); err != nil {
				c.notifier(fmt.Errorf("response handle failed: %w", err))
				if needRetry {
					connection.conn.SendMessage(msgInfo.msg)
				}
				return
			}
			if msgInfo.isSub {
				kind, _ := connection.subKinds.Load(data.GetId())
				if err := connection.conn.SubConfirm(kind.(structs.SubKind)); err != nil {
					c.notifier(err)
				}
			} else {
				// при подписке нельзя удалять инфу, чтобы была возможной переподписка при падении коннекта
				connection.messages.Delete(data.GetId())
			}
		}
	}
}

func (c *WsClient) SendMessage(connInd int, msg WsMessage, handler WsMessageHandler) error {
	if connInd < 0 || connInd >= len(c.connections) {
		return ConnIndexOutOfRangeErr
	}
	connInfo := c.connections[connInd]
	id := atomic.AddUint64(&connInfo.count, 1)
	msg.SetId(id)
	connInfo.messages.Store(id, &messageInfo{
		msg:     msg,
		handler: handler,
	})
	connInfo.conn.SendMessage(msg)
	return nil
}

func (c *WsClient) Sub(connInd int, kind structs.SubKind, req WsMessage, handler WsMessageHandler) error {
	if connInd < 0 || connInd >= len(c.connections) {
		return ConnIndexOutOfRangeErr
	}
	connInfo := c.connections[connInd]
	id := atomic.AddUint64(&connInfo.count, 1)
	req.SetId(id)
	connInfo.messages.Store(id, &messageInfo{
		msg:     req,
		handler: handler,
		isSub:   true,
	})
	connInfo.subKinds.Store(id, kind)
	return connInfo.conn.Sub(kind, req)
}

func (c *WsClient) IsAlive() bool {
	for _, connection := range c.connections {
		if !connection.conn.IsAlive() {
			return false
		}
	}
	return true
}

func (c *WsClient) Stop() error {
	for _, info := range c.connections {
		info.conn.Close()
	}
	return c.client.stop()
}
