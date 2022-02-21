package apiclient

import (
	"bytes"
	"context"
	"fmt"
	"github.com/loyal-inform/sdk-go/util/consts"
	"google.golang.org/protobuf/proto"
	"io/ioutil"
	"net/http"
	"time"
)

// HttpClient - клиент для вызовов с помощью http-запросов
type HttpClient struct {
	*client
	httpClient  *http.Client
	baseUrl     string
	fillVersion func(http.Header)
}

// HttpMessage - сообщение для отправки другому сервису
type HttpMessage struct {
	Message proto.Message // сообщение
	Url     string        // часть адреса(path), то есть протокол и хост сюда не включены
}

// HttpMessageHandler - сигнатура функции обработки ответа на http запрос
type HttpMessageHandler func(msg []byte) (needRetry bool, err error)

// NewHttpClient - функция создания http клиента
func NewHttpClient(baseUrl string, accessToken string, accessExpiresAt int64,
	refreshToken string, refreshExpiresAt int64, fillVersion func(http.Header),
	refresh refreshFunc, notifier errorNotifier) (*HttpClient, error) {
	c, err := newClient(accessToken, accessExpiresAt, refreshToken, refreshExpiresAt, refresh, notifier)
	if err != nil {
		return nil, err
	}
	return &HttpClient{
		client:      c,
		baseUrl:     baseUrl,
		httpClient:  &http.Client{},
		fillVersion: fillVersion,
	}, nil
}

func (c *HttpClient) Start() error {
	return c.client.start()
}

func (c *HttpClient) MakeRequest(ctx context.Context, msg *HttpMessage, handler HttpMessageHandler) {
	data, err := proto.Marshal(msg.Message)
	if err != nil {
		c.notifier(fmt.Errorf("marshal request failed: %w", err))
		return
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s%s", c.baseUrl, msg.Url), bytes.NewReader(data))
	if err != nil {
		c.notifier(fmt.Errorf("create request failed: %w", err))
		return
	}
	r.Header.Set(consts.HeaderContentType, consts.ProtoContentType)
	r.Header.Set(consts.AuthHeader, fmt.Sprintf("%s%s", consts.TokenStart, c.getAccessToken()))
	if c.fillVersion != nil {
		c.fillVersion(r.Header)
	}
	resp, err := c.httpClient.Do(r)
	if err != nil {
		c.notifier(fmt.Errorf("do request failed: %w", err))
		return
	}
	defer resp.Body.Close()
	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.notifier(fmt.Errorf("read response failed: %w", err))
		return
	}
	if needRetry, err := handler(respData); err != nil {
		c.notifier(fmt.Errorf("handle response: %w", err))
		if needRetry {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			c.MakeRequest(ctx, msg, handler)
			cancel()
		}
	}
}

func (c *HttpClient) Stop() error {
	return c.client.stop()
}
