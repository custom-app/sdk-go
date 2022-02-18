package apiclient

import (
	"errors"
	"fmt"
	"github.com/loyal-inform/sdk-go/service/job/ws"
	"github.com/loyal-inform/sdk-go/service/ws/conn"
	"github.com/loyal-inform/sdk-go/service/ws/opts"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"
	"sync"
)

var (
	AddressRequiredErr     = errors.New("address required")
	ConnIndexOutOfRangeErr = errors.New("conn ind out of range")
	UnhandledMessageErr    = errors.New("unhandled message")
)

type WsMessageKind int

const (
	MessageKindAuth = WsMessageKind(iota)
	MessageKindSub
	MessageKindResponse
)

type ConnectionInfo struct {
	Address     string
	ConnOptions *opts.ClientPrivateConnOptions
	SubHandler  WsMessageHandler
	conn        *conn.ClientPrivateConn
	worker      *ws.ClientPrivateWorker
	count       *atomic.Uint64
	messages    *sync.Map
}

type WsMessageHandler func(msg proto.Message) (needRetry bool, err error)

type WsMsgParser func([]byte) (WsMessageKind, WsMessage, error)

type messageInfo struct {
	handler WsMessageHandler
	msg     WsMessage
}

type WsMessage interface {
	proto.Message
	GetId() uint64
	SetId(id uint64)
}

type WsClient struct {
	*client
	connections []*ConnectionInfo
	msgParser   WsMsgParser
}

func NewWsClient(connections []*ConnectionInfo, accessToken string, accessExpiresAt int64,
	refreshToken string, refreshExpiresAt int64, refresh refreshFunc, notifier errorNotifier,
	msgParser WsMsgParser) (*WsClient, error) {
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
		connection.count, connection.messages = atomic.NewUint64(0), &sync.Map{}
		connection.conn, err = conn.NewClientPrivateConnWithToken(connection.Address, c.getAccessToken(), connection.ConnOptions)
		if err != nil {
			return err
		}
		connection.worker = ws.NewClientPrivateWorker(connection.conn.ReceiveBuf(), c.handleMessage(connection))
		go connection.worker.Run()
		if err := connection.conn.Auth(); err != nil {
			return err
		}
	}
	return nil
}

func (c *WsClient) handleMessage(connection *ConnectionInfo) ws.ClientPrivateHandler {
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
				if _, err := connection.SubHandler(data); err != nil {
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
			connection.messages.Delete(data.GetId())
		}
	}
}

func (c *WsClient) SendMessage(connInd int, msg WsMessage, handler WsMessageHandler) error {
	if connInd < 0 || connInd >= len(c.connections) {
		return ConnIndexOutOfRangeErr
	}
	connInfo := c.connections[connInd]
	id := connInfo.count.Add(1)
	msg.SetId(id)
	connInfo.messages.Store(id, &messageInfo{
		msg:     msg,
		handler: handler,
	})
	connInfo.conn.SendMessage(msg)
	return nil
}

func (c *WsClient) Stop() error {
	for _, info := range c.connections {
		info.conn.Close()
	}
	return c.client.stop()
}
