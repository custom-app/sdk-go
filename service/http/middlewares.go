package http

import (
	"context"
	"errors"
	"github.com/loyal-inform/sdk-go/auth/basic"
	"github.com/loyal-inform/sdk-go/auth/jwt/multiple"
	"github.com/loyal-inform/sdk-go/auth/jwt/single"
	"github.com/loyal-inform/sdk-go/structs"
	"github.com/loyal-inform/sdk-go/util/consts"
	"google.golang.org/protobuf/proto"
	"net/http"
	"strconv"
	"strings"
)

const (
	AuthHeader       = "Authorization"
	TokenStart       = "Bearer "
	TokenStartInd    = len(TokenStart)
	VersionDelimiter = ":"
)

var (
	PermissionDenied   = errors.New("permission denied")
	MissingCredentials = errors.New("missing credentials")
)

type VersionChecker func(platform structs.Platform, versions []string) (int, proto.Message)

func VersionMiddleware(header string, checker VersionChecker) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			value := r.Header.Get(header)
			parts := strings.Split(value, VersionDelimiter)
			if len(parts) < 2 {
				code, e := checker(0, nil)
				SendResponseWithContentType(w, r, code, e)
				return
			}
			p, err := strconv.ParseInt(parts[0], 10, 32)
			if err != nil {
				code, e := checker(0, nil)
				SendResponseWithContentType(w, r, code, e)
				return
			}
			if code, e := checker(structs.Platform(p), parts[1:]); e != nil {
				SendResponseWithContentType(w, r, code, e)
				return
			}
			ctx := context.WithValue(r.Context(), consts.PlatformCtxKey, structs.Platform(p))
			ctx = context.WithValue(r.Context(), consts.VersionsCtxKey, parts[1:])
			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type AuthKind struct {
	Basic, AuthToken, RefreshToken, MultipleTokens bool
}

type AuthErrorMapper func(error) (int, proto.Message)

func AuthMiddleware(accepted AuthKind, errorMapper AuthErrorMapper, roles ...structs.Role) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := r.Header.Get(AuthHeader)
			isToken := strings.HasPrefix(a, TokenStart)
			login, password, ok := r.BasicAuth()
			platform, _ := r.Context().Value(consts.PlatformCtxKey).(structs.Platform)
			versions, _ := r.Context().Value(consts.VersionsCtxKey).([]string)
			var (
				acc    *structs.Account
				number int64 = -1
				err    error
			)
			if ok && accepted.Basic {
				acc, err = basic.Auth(r.Context(), login, password, platform, versions)
			} else if isToken && accepted.MultipleTokens {
				if accepted.AuthToken {
					acc, number, err = multiple.Auth(r.Context(), a[TokenStartInd:], structs.PurposeAccess, platform, versions)
				} else {
					acc, number, err = multiple.Auth(r.Context(), a[TokenStartInd:], structs.PurposeRefresh, platform, versions)
				}
			} else if isToken && !accepted.MultipleTokens {
				if accepted.AuthToken {
					acc, err = single.Auth(a[TokenStartInd:], structs.PurposeAccess, platform, versions)
				} else {
					acc, err = single.Auth(a[TokenStartInd:], structs.PurposeRefresh, platform, versions)
				}
			} else {
				err = MissingCredentials
			}
			if err != nil {
				code, e := errorMapper(PermissionDenied)
				SendResponseWithContentType(w, r, code, e)
				return
			}
			var ctx context.Context
			for _, role := range roles {
				if role == acc.Role {
					ctx = context.WithValue(r.Context(), consts.AccountCtxKey, acc)
					if number != -1 {
						ctx = context.WithValue(ctx, consts.TokenNumberCtxKey, number)
					}
					break
				}
			}
			if ctx == nil {
				code, e := errorMapper(PermissionDenied)
				SendResponseWithContentType(w, r, code, e)
				return
			}
			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
