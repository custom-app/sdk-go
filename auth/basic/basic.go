package basic

import (
	"errors"
	"fmt"
	"github.com/loyal-inform/sdk-go/structs"
)

type AuthFunc func(login, password string, platform structs.Platform, versions []string) (structs.Account, error)

var defaultAuth AuthFunc

var (
	AuthFailedErr = errors.New("auth failed")
)

func Auth(login, password string, platform structs.Platform, versions []string) (structs.Account, error) {
	if defaultAuth == nil {
		return nil, fmt.Errorf("unset func")
	}
	return defaultAuth(login, password, platform, versions)
}
