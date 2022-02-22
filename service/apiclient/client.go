// Package apiclient - пакет API клиента для использования в сервисах (для возможности вызова service to service)
//
// Включает в себя автоматическое обновление jwt токенов через сервис авторизации
package apiclient

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	RefreshFuncIsRequired = errors.New("refresh function is required")
)

type refreshFunc func(ctx context.Context, token string) (
	authToken string, authExpiresAt int64, refreshToken string, refreshExpiresAt int64, err error)

type errorNotifier func(err error)

type client struct {
	refresh                           refreshFunc
	notifier                          errorNotifier
	tokenLock                         *sync.Mutex
	accessToken, refreshToken         string
	accessExpiresAt, refreshExpiresAt int64
	refreshStopCh                     chan struct{}
	updateTokenCallback               func(string)
}

func newClient(accessToken string, accessExpiresAt int64, refreshToken string, refreshExpiresAt int64,
	refresh refreshFunc, updateTokenCallback func(string), notifier errorNotifier) (*client, error) {
	if refresh == nil {
		return nil, RefreshFuncIsRequired
	}
	return &client{
		refresh:             refresh,
		accessToken:         accessToken,
		refreshToken:        refreshToken,
		accessExpiresAt:     accessExpiresAt,
		refreshExpiresAt:    refreshExpiresAt,
		tokenLock:           &sync.Mutex{},
		refreshStopCh:       make(chan struct{}),
		notifier:            notifier,
		updateTokenCallback: updateTokenCallback,
	}, nil
}

func (c *client) start() error {
	go c.refreshTokens()
	return nil
}

func (c *client) getAccessToken() string {
	c.tokenLock.Lock()
	res := c.accessToken
	c.tokenLock.Unlock()
	return res
}

func (c *client) getRefreshToken() string {
	c.tokenLock.Lock()
	res := c.refreshToken
	c.tokenLock.Unlock()
	return res
}

func (c *client) refreshTokens() {
	for {
		toWait := time.Unix(c.accessExpiresAt/1000, 0).Sub(time.Now()) / 2
		select {
		case <-time.After(toWait):
			c.tokenLock.Lock()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			accessToken, accessExpiresAt, refreshToken, refreshExpiresAt, err :=
				c.refresh(ctx, c.refreshToken)
			cancel()
			if err != nil {
				if c.notifier != nil {
					c.notifier(err)
				}
			} else {
				c.accessToken, c.accessExpiresAt, c.refreshToken, c.refreshExpiresAt =
					accessToken, accessExpiresAt, refreshToken, refreshExpiresAt
			}
			if c.updateTokenCallback != nil {
				c.updateTokenCallback(accessToken)
			}
			c.tokenLock.Unlock()
		case <-c.refreshStopCh:
			return
		}
	}
}

func (c *client) stop() error {
	c.refreshStopCh <- struct{}{}
	return nil
}
