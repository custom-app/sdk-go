package basic

import (
	"context"
	"fmt"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"sync"
)

var (
	providerLock = &sync.RWMutex{}
)

type AuthProvider interface {
	Auth(ctx context.Context, login, password string, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, error)
	AuthWithInfo(ctx context.Context, login, password string, platform structs.Platform,
		versions []string, disabled ...structs.Role) (*structs.Account, proto.Message, error)
	Logout(ctx context.Context, role structs.Role, id int64) error
}

var defaultAuth AuthProvider

func Auth(ctx context.Context, login, password string, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, error) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	if defaultAuth == nil {
		return nil, fmt.Errorf("unset provider")
	}
	return defaultAuth.Auth(ctx, login, password, platform, versions, disabled...)
}

func AuthWithInfo(ctx context.Context, login, password string, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, proto.Message, error) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	if defaultAuth == nil {
		return nil, nil, fmt.Errorf("unset provider")
	}
	return defaultAuth.AuthWithInfo(ctx, login, password, platform, versions, disabled...)
}

func Logout(ctx context.Context, role structs.Role, id int64) error {
	providerLock.RLock()
	defer providerLock.RUnlock()
	return defaultAuth.Logout(ctx, role, id)
}

func SetDefaultAuth(f AuthProvider) {
	providerLock.Lock()
	defaultAuth = f
	providerLock.Unlock()
}
