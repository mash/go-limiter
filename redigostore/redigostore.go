package redigostore

import (
	"time"

	"github.com/garyburd/redigo/redis"
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
	conn.Send("INCR", key)
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
