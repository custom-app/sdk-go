// Package time - используется для возможности замены функции получения текущего времени.
//
// Полезно для использования в тестах
package time

import "time"

var (
	nowFunc = time.Now
)

func SetNowFunc(f func() time.Time) {
	nowFunc = f
}

func Now() time.Time {
	return nowFunc()
}
