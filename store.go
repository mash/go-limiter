package limiter

import "time"

type Store interface {
	Get(key string, d time.Duration) (int64, error)
}
