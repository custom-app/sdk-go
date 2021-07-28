package auth

import "errors"

var (
	FailedAuthErr       = errors.New("auth failed")
	PermissionDeniedErr = errors.New("permission denied")
)
