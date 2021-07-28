package basic

import (
	"context"
	"errors"
	"fmt"
	"github.com/loyal-inform/sdk-go/structs"
)

type AuthFunc func(ctx context.Context, login, password string, platform structs.Platform, versions []string) (*structs.Account, error)

var defaultAuth AuthFunc

var (
	AuthFailedErr = errors.New("auth failed")
)

func Auth(ctx context.Context, login, password string, platform structs.Platform, versions []string) (*structs.Account, error) {
	if defaultAuth == nil {
		return nil, fmt.Errorf("unset func")
	}
	return defaultAuth(ctx, login, password, platform, versions)
}

func SetDefaultAuth(f AuthFunc) {
	defaultAuth = f
}
