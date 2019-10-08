package redigostore

import (
	"time"

	"github.com/gomodule/redigo/redis"
)

type pool interface {
	Get() redis.Conn
	Close() error
}

type store struct {
	pool pool
}

// Conforms to limiter.Store interface
func (s store) Get(key string, expires time.Duration) (int64, error) {
	conn := s.pool.Get()
	defer conn.Close()

	conn.Send("MULTI")

	// https://redis.io/commands/incr
	// If the key does not exist, it is set to 0 before performing the operation
	conn.Send("INCR", key)

	// https://redis.io/commands/expire
	// For instance, incrementing the value of a key with INCR, ...
	// are all operations that will leave the timeout untouched.
	conn.Send("EXPIRE", key, int(expires.Seconds()))

	v, err := redis.Int64s(conn.Do("EXEC"))
	if err != nil {
		return 0, err
	}
	return v[0], nil
}

func New(pool pool) store {
	return store{pool: pool}
}
