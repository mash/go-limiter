package redigo

import "github.com/garyburd/redigo/redis"

type RedigoAdaptor struct {
	redis.Conn
}

func NewRedigoAdaptor(c redis.Conn) RedigoAdaptor {
	return RedigoAdaptor{Conn: c}
}

func (r RedigoAdaptor) Get(key string) (uint64, error) {
	u, e := redis.Uint64(r.Conn.Do("GET", key))
	if e == redis.ErrNil {
		return 0, nil
	}
	return u, e
}

func (r RedigoAdaptor) Incrx(key string, seconds int) error {
	r.Conn.Send("MULTI")
	r.Conn.Send("INCR", key)
	r.Conn.Send("EXPIRE", key, seconds)
	_, err := r.Conn.Do("EXEC")
	return err
}
