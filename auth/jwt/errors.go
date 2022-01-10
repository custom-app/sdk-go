// Package jwt содержит общие ошибки и структуры для авторизации с помощью jwt.
//
// О том, как работает jwt - https://habr.com/ru/post/340146/.
package jwt

import "errors"

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
