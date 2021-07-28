package jwt

import "errors"

var (
	AuthFailedErr       = errors.New("auth failed")
	ParseTokenErr       = errors.New("parse token failed")
	ExpiredToken        = errors.New("expired token")
	InvalidTokenPurpose = errors.New("invalid token purpose")
	InvalidToken        = errors.New("invalid token")
)
