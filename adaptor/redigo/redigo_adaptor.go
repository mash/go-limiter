package redigo

import (
	"github.com/garyburd/redigo/redis"
	"github.com/mash/go-limiter/adaptor"
)

type RedisPool struct {
	pool redis.Pool
}

func NewRedigoAdaptor(pool redis.Pool) RedisPool {
	return RedisPool{pool: pool}
}

// caller should defer conn.Close()
func (r RedisPool) Borrow() adaptor.RedisClient {
	return redisClient{
		conn: r.pool.Get(),
	}
}

type redisClient struct {
	conn redis.Conn
}

func (r redisClient) Close() error {
	return r.conn.Close()
}

func (r redisClient) Get(key string) (uint64, error) {
	u, e := redis.Uint64(r.conn.Do("GET", key))
	if e == redis.ErrNil {
		return 0, nil
	}
	return u, e
}

func (r redisClient) Incrx(key string, seconds int) error {
	r.conn.Send("MULTI")
	r.conn.Send("INCR", key)
	r.conn.Send("EXPIRE", key, seconds)
	_, err := r.conn.Do("EXEC")
	return err
}
