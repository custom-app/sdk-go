package jwt

import "errors"

var (
	ParseTokenErr          = errors.New("parse token failed")
	ExpiredTokenErr        = errors.New("expired token")
	InvalidTokenPurposeErr = errors.New("invalid token purpose")
	InvalidTokenErr        = errors.New("invalid token")
)
