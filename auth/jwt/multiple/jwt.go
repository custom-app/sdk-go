package multiple

import (
	"context"
	"fmt"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
)

type AuthProvider interface {
	Auth(ctx context.Context, token string, purpose structs.Purpose, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, int64, error)
	AuthWithInfo(ctx context.Context, token string, purpose structs.Purpose, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, int64, proto.Message, error)
	Logout(ctx context.Context, role structs.Role, id int64) error
}

var defaultAuth AuthProvider

func Auth(ctx context.Context, token string, purpose structs.Purpose, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, int64, error) {
	if defaultAuth == nil {
		return nil, 0, fmt.Errorf("unset provider")
	}
	return defaultAuth.Auth(ctx, token, purpose, platform, versions, disabled...)
}

func AuthWithInfo(ctx context.Context, token string, purpose structs.Purpose, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, int64, proto.Message, error) {
	if defaultAuth == nil {
		return nil, 0, nil, fmt.Errorf("unset provider")
	}
	return defaultAuth.AuthWithInfo(ctx, token, purpose, platform, versions, disabled...)
}

func Logout(ctx context.Context, role structs.Role, id int64) error {
	return defaultAuth.Logout(ctx, role, id)
}

func SetDefaultAuth(f AuthProvider) {
	defaultAuth = f
}
