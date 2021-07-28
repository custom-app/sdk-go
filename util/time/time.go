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
