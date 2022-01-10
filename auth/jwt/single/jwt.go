// Package single - реализация jwt с одним единовременным токеном для пользователя.
//
// Примеры использования приведены в пакетах с реализациями интерфейса AuthProvider.
package single

import (
	"context"
	"fmt"
	"github.com/loyal-inform/sdk-go/auth/jwt"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"sync"
)

var (
	providerLock = &sync.RWMutex{}
)

// AuthProvider - интерфейс провайдера авторизации по jwt
type AuthProvider interface {
	// Auth - получение аккаунта по jwt-токену
	Auth(ctx context.Context, token string, purpose jwt.Purpose, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, error)
	// AuthWithInfo - получение аккаунта и полного ответа(полная структура аккаунта,
	// упакованная в ответ на запрос согласно определению API) на запрос авторизации по jwt-токену
	AuthWithInfo(ctx context.Context, token string, purpose jwt.Purpose, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, proto.Message, error)
	// CreateTokens - создание новых токенов для пользователя
	CreateTokens(ctx context.Context, role structs.Role, id int64) (string, int64, string, int64, error)
	// Logout - удаление токенов
	Logout(ctx context.Context, role structs.Role, id int64) error
}

var defaultAuth AuthProvider

// Auth - вызов метода Auth у провайдера по умолчанию
func Auth(ctx context.Context, token string, purpose jwt.Purpose, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, error) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	if defaultAuth == nil {
		return nil, fmt.Errorf("unset provider")
	}
	return defaultAuth.Auth(ctx, token, purpose, platform, versions, disabled...)
}

// AuthWithInfo - вызов метода AuthWithInfo у провайдера по умолчанию
func AuthWithInfo(ctx context.Context, token string, purpose jwt.Purpose, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, proto.Message, error) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	if defaultAuth == nil {
		return nil, nil, fmt.Errorf("unset provider")
	}
	return defaultAuth.AuthWithInfo(ctx, token, purpose, platform, versions, disabled...)
}

// CreateTokens - вызов метода CreateTokens у провайдера по умолчанию
func CreateTokens(ctx context.Context, role structs.Role, id int64) (string, int64, string, int64, error) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	if defaultAuth == nil {
		return "", 0, "", 0, fmt.Errorf("unset provider")
	}
	return defaultAuth.CreateTokens(ctx, role, id)
}

// Logout - вызов метода Logout у провайдера по умолчанию
func Logout(ctx context.Context, role structs.Role, id int64) error {
	providerLock.RLock()
	defer providerLock.RUnlock()
	return defaultAuth.Logout(ctx, role, id)
}

// SetDefaultAuth - установка провайдера по умолчанию
func SetDefaultAuth(f AuthProvider) {
	providerLock.Lock()
	defaultAuth = f
	providerLock.Unlock()
}
