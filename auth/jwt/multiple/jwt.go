package multiple

import (
	"fmt"
	"github.com/loyal-inform/sdk-go/structs"
)

type AuthFunc func(token string, purpose structs.Purpose, platform structs.Platform, versions []string) (structs.Account, int64, error)

var defaultAuth AuthFunc

func Auth(token string, purpose structs.Purpose, platform structs.Platform, versions []string) (structs.Account, int64, error) {
	if defaultAuth == nil {
		return nil, 0, fmt.Errorf("unset func")
	}
	return defaultAuth(token, purpose, platform, versions)
}
