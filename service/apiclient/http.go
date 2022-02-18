package apiclient

import (
	"bytes"
	"fmt"
	"github.com/loyal-inform/sdk-go/util/consts"
	"google.golang.org/protobuf/proto"
	"io/ioutil"
	"net/http"
)

type HttpClient struct {
	*client
	httpClient *http.Client
	baseUrl    string
}

type HttpMessage struct {
	Message proto.Message
	Url     string
}

type HttpMessageHandler func(msg []byte) (needRetry bool, err error)

func NewHttpClient(baseUrl string, accessToken string, accessExpiresAt int64,
	refreshToken string, refreshExpiresAt int64, refresh refreshFunc, notifier errorNotifier) (*HttpClient, error) {
	c, err := newClient(accessToken, accessExpiresAt, refreshToken, refreshExpiresAt, refresh, notifier)
	if err != nil {
		return nil, err
	}
	return &HttpClient{
		client:     c,
		baseUrl:    baseUrl,
		httpClient: &http.Client{},
	}, nil
}

func (c *HttpClient) Start() error {
	return c.client.start()
}

func (c *HttpClient) MakeRequest(msg *HttpMessage, handler HttpMessageHandler) {
	data, err := proto.Marshal(msg.Message)
	if err != nil {
		c.notifier(fmt.Errorf("marshal request failed: %w", err))
		return
	}
	r, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s%s", c.baseUrl, msg.Url), bytes.NewReader(data))
	if err != nil {
		c.notifier(fmt.Errorf("create request failed: %w", err))
		return
	}
	r.Header.Set(consts.HeaderContentType, consts.ProtoContentType)
	r.Header.Set(consts.AuthHeader, fmt.Sprintf("%s%s", consts.TokenStart, c.getAccessToken()))
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
			c.MakeRequest(msg, handler)
		}
	}
}

func (c *HttpClient) Stop() error {
	return c.client.stop()
}
