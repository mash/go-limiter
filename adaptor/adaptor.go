package adaptor

import "io"

type RedisPool interface {
	Borrow() RedisClient
}

type RedisClient interface {
	io.Closer
	Get(key string) (uint64, error)
	Incrx(key string, seconds int) error
}
