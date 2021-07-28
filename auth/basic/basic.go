package basic

import (
	"context"
	"fmt"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
)

type AuthProvider interface {
	Auth(ctx context.Context, login, password string, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, error)
	AuthWithInfo(ctx context.Context, login, password string, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, proto.Message, error)
}

var defaultAuth AuthProvider

func Auth(ctx context.Context, login, password string, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, error) {
	if defaultAuth == nil {
		return nil, fmt.Errorf("unset provider")
	}
	return defaultAuth.Auth(ctx, login, password, platform, versions, disabled...)
}

func AuthWithInfo(ctx context.Context, login, password string, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, proto.Message, error) {
	if defaultAuth == nil {
		return nil, nil, fmt.Errorf("unset provider")
	}
	return defaultAuth.AuthWithInfo(ctx, login, password, platform, versions, disabled...)
}

func SetDefaultAuth(f AuthProvider) {
	defaultAuth = f
}
