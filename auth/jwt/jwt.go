// Package jwt - реализация jwt с возможностью нескольких сессий.
//
// Также пакет содержит общие ошибки и структуры для авторизации с помощью jwt.
//
// О том, как работает jwt - https://habr.com/ru/post/340146/.
//
// Примеры использования приведены в пакетах с реализациями интерфейса AuthProvider.
package jwt

import (
	"context"
	"errors"
	"fmt"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"sync"
)

type Purpose int32 // enum для типа jwt-токена

const (
	PurposeAccess  = Purpose(iota) // access токен
	PurposeRefresh                 // refresh токен
)

var (
	ParseTokenErr          = errors.New("parse token failed")    // ParseTokenErr - не удалось распарсить токен
	ExpiredTokenErr        = errors.New("expired token")         // ExpiredTokenErr - токен просрочен
	InvalidTokenPurposeErr = errors.New("invalid token purpose") // InvalidTokenPurposeErr - некорректный purpose у токена
	InvalidTokenErr        = errors.New("invalid token")         // InvalidTokenErr - не удалось распарсить один из claim'ов токена
)

var (
	providerLock = &sync.RWMutex{}
)

// AuthProvider - интерфейс провайдера авторизации по jwt
type AuthProvider interface {
	// Auth - получение аккаунта по логину/паролю
	Auth(ctx context.Context, token string, purpose Purpose, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, int64, error)
	// AuthWithInfo - получение аккаунта и полного ответа(полная структура аккаунта,
	// упакованная в ответ на запрос согласно определению API) на запрос авторизации по jwt-токену
	AuthWithInfo(ctx context.Context, token string, purpose Purpose, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, int64, proto.Message, error)
	// Logout - удаление токенов
	Logout(ctx context.Context, role structs.Role, id int64) error
	// CreateTokens - создание новых токенов для пользователя
	CreateTokens(ctx context.Context, role structs.Role, id int64) (string, int64, string, int64, error)
	// ReCreateTokens - создание новых токенов для фиксированной сессии пользователя
	ReCreateTokens(ctx context.Context, role structs.Role, id, number int64) (string, int64, string, int64, error)
	// DropTokens - удаление токена
	DropTokens(ctx context.Context, role structs.Role, id, number int64) error
	// DropOldTokens - удаление просроченных токенов
	DropOldTokens(ctx context.Context, timestamp int64) error
}

var defaultAuth AuthProvider

// Auth - вызов метода Auth у провайдера по умолчанию
func Auth(ctx context.Context, token string, purpose Purpose, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, int64, error) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	if defaultAuth == nil {
		return nil, 0, fmt.Errorf("unset provider")
	}
	return defaultAuth.Auth(ctx, token, purpose, platform, versions, disabled...)
}

// AuthWithInfo - вызов метода AuthWithInfo у провайдера по умолчанию
func AuthWithInfo(ctx context.Context, token string, purpose Purpose, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, int64, proto.Message, error) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	if defaultAuth == nil {
		return nil, 0, nil, fmt.Errorf("unset provider")
	}
	return defaultAuth.AuthWithInfo(ctx, token, purpose, platform, versions, disabled...)
}

// Logout - вызов метода Logout у провайдера по умолчанию
func Logout(ctx context.Context, role structs.Role, id int64) error {
	providerLock.RLock()
	defer providerLock.RUnlock()
	return defaultAuth.Logout(ctx, role, id)
}

// CreateTokens - вызов метода CreateTokens у провайдера по умолчанию
func CreateTokens(ctx context.Context, role structs.Role, id int64) (string, int64, string, int64, error) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	return defaultAuth.CreateTokens(ctx, role, id)
}

// ReCreateTokens - вызов метода ReCreateTokens у провайдера по умолчанию
func ReCreateTokens(ctx context.Context, role structs.Role, id, number int64) (string, int64, string, int64, error) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	return defaultAuth.ReCreateTokens(ctx, role, id, number)
}

// DropTokens - вызов метода DropTokens у провайдера по умолчанию
func DropTokens(ctx context.Context, role structs.Role, id, number int64) error {
	providerLock.RLock()
	defer providerLock.RUnlock()
	return defaultAuth.DropTokens(ctx, role, id, number)
}

// DropOldTokens - вызов метода DropOldTokens у провайдера по умолчанию
func DropOldTokens(ctx context.Context, timestamp int64) error {
	providerLock.RLock()
	defer providerLock.RUnlock()
	return defaultAuth.DropOldTokens(ctx, timestamp)
}

// SetDefaultAuth - установка провайдера по умолчанию
func SetDefaultAuth(f AuthProvider) {
	providerLock.Lock()
	defaultAuth = f
	providerLock.Unlock()
}
