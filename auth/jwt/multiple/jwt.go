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
	CreateTokens(ctx context.Context, role structs.Role, id int64) (string, int64, string, int64, error)
	ReCreateTokens(ctx context.Context, role structs.Role, id, number int64) (string, int64, string, int64, error)
	DropTokens(ctx context.Context, role structs.Role, id, number int64) error
	DropOldTokens(ctx context.Context, timestamp int64) error
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

func CreateTokens(ctx context.Context, role structs.Role, id int64) (string, int64, string, int64, error) {
	return defaultAuth.CreateTokens(ctx, role, id)
}

func ReCreateTokens(ctx context.Context, role structs.Role, id, number int64) (string, int64, string, int64, error) {
	return defaultAuth.ReCreateTokens(ctx, role, id, number)
}

func DropTokens(ctx context.Context, role structs.Role, id, number int64) error {
	return defaultAuth.DropTokens(ctx, role, id, number)
}

func DropOldTokens(ctx context.Context, timestamp int64) error {
	return defaultAuth.DropOldTokens(ctx, timestamp)
}

func SetDefaultAuth(f AuthProvider) {
	defaultAuth = f
}
