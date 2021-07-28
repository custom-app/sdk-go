package multiple

import (
	"context"
	"fmt"
	"github.com/loyal-inform/sdk-go/structs"
)

type AuthFunc func(ctx context.Context, token string, purpose structs.Purpose,
	platform structs.Platform, versions []string) (*structs.Account, int64, error)

var defaultAuth AuthFunc

func Auth(ctx context.Context, token string, purpose structs.Purpose,
	platform structs.Platform, versions []string) (*structs.Account, int64, error) {
	if defaultAuth == nil {
		return nil, 0, fmt.Errorf("unset func")
	}
	return defaultAuth(ctx, token, purpose, platform, versions)
}

func SetDefaultAuth(f AuthFunc) {
	defaultAuth = f
}
