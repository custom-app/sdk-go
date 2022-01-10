// Package auth - пакет с общими ошибками и структурами для работы авторизации
// Здесь собраны разные реализации авторизации.
//
// Они все сделаны в виде провайдеров, но провайдер по умолчанию не заполняется реализаций с моками
package auth

import "errors"

var (
	FailedAuthErr       = errors.New("auth failed")       // неудачная авторизация
	PermissionDeniedErr = errors.New("permission denied") // для роли аккаунта нет доступа
)
