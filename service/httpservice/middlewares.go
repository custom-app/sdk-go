package httpservice

import (
	"context"
	"errors"
	"github.com/custom-app/sdk-go/auth"
	"github.com/custom-app/sdk-go/auth/basic"
	"github.com/custom-app/sdk-go/auth/jwt"
	"github.com/custom-app/sdk-go/structs"
	"github.com/custom-app/sdk-go/util/consts"
	"google.golang.org/protobuf/proto"
	"net/http"
	"strconv"
	"strings"
)

var (
	MissingCredentials = errors.New("missing credentials")
)

func ParseVersionHeader(header http.Header, key string) (structs.Platform, []string) {
	value := header.Get(key)
	parts := strings.Split(value, consts.VersionDelimiter)
	if len(parts) < 2 {
		return 0, nil
	}
	p, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return 0, nil
	}
	return structs.Platform(p), parts[1:]
}

type VersionChecker func(platform structs.Platform, versions []string) (int, proto.Message)

func VersionMiddleware(header string, checker VersionChecker) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			platform, versions := ParseVersionHeader(r.Header, header)
			if code, e := checker(platform, versions); e != nil {
				SendResponseWithContentType(w, r, code, e)
				return
			}
			ctx := context.WithValue(r.Context(), consts.PlatformCtxKey, platform)
			ctx = context.WithValue(ctx, consts.VersionsCtxKey, versions)
			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type AuthKind struct {
	Basic, AuthToken, RefreshToken, Cookie bool
	CookieName                             string
}

type AuthErrorMapper func(error) (int, proto.Message)

func AuthMiddleware(accepted AuthKind, errorMapper AuthErrorMapper, roles ...structs.Role) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := r.Header.Get(consts.AuthHeader)
			cookieValue, cookieErr := r.Cookie(accepted.CookieName)
			isToken := strings.HasPrefix(a, consts.TokenStart)
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
			} else if isToken {
				if accepted.AuthToken {
					acc, number, err = jwt.Auth(r.Context(), a[consts.TokenStartInd:],
						jwt.PurposeAccess, platform, versions)
				} else {
					acc, number, err = jwt.Auth(r.Context(), a[consts.TokenStartInd:],
						jwt.PurposeRefresh, platform, versions)
				}
			} else if accepted.Cookie && cookieErr == nil && strings.HasPrefix(cookieValue.Value, consts.TokenStart) {
				if accepted.AuthToken {
					acc, number, err = jwt.Auth(r.Context(), cookieValue.Value[consts.TokenStartInd:],
						jwt.PurposeAccess, platform, versions)
				} else if accepted.RefreshToken {
					acc, number, err = jwt.Auth(r.Context(), cookieValue.Value[consts.TokenStartInd:],
						jwt.PurposeRefresh, platform, versions)
				} else {
					err = MissingCredentials
				}
			} else {
				err = MissingCredentials
			}
			if err != nil {
				code, e := errorMapper(err)
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
				code, e := errorMapper(auth.PermissionDeniedErr)
				SendResponseWithContentType(w, r, code, e)
				return
			}
			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
