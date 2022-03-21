// Package basic - пакет с реализацией авторизации по логин/паролю.
//
// Подразумевается, что пароли в базе данных хранятся в виде SHA256 хэшей и сами пароли тоже присылаются хэшированными.
//
// Примеры использования приведены в пакетах с реализациями интерфейса AuthProvider.
package basic

import (
	"context"
	"fmt"
	"github.com/custom-app/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"sync"
)

var (
	providerLock = &sync.RWMutex{}
)

// AuthProvider - интерфейс провайдера авторизации по логин/паролю
type AuthProvider interface {
	// Auth - получение аккаунта по логину/паролю
	Auth(ctx context.Context, login, password string, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, error)
	// AuthWithInfo - получение аккаунта и полного ответа(полная структура аккаунта,
	// упакованная в ответ на запрос согласно определению API) на запрос авторизации по логину/паролю
	AuthWithInfo(ctx context.Context, login, password string, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, proto.Message, error)
	Logout(ctx context.Context, role structs.Role, id int64) error
}

var defaultAuth AuthProvider

// Auth - вызов метода Auth у провайдера по умолчанию
func Auth(ctx context.Context, login, password string, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, error) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	if defaultAuth == nil {
		return nil, fmt.Errorf("unset provider")
	}
	return defaultAuth.Auth(ctx, login, password, platform, versions, disabled...)
}

// AuthWithInfo - вызов метода AuthWithInfo у провайдера по умолчанию
func AuthWithInfo(ctx context.Context, login, password string, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, proto.Message, error) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	if defaultAuth == nil {
		return nil, nil, fmt.Errorf("unset provider")
	}
	return defaultAuth.AuthWithInfo(ctx, login, password, platform, versions, disabled...)
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
