package main

import (
	"log"
	"net/http"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/mash/go-limiter"
	"github.com/mash/go-limiter/adaptor/redigo"
)

func main() {
	pool := redis.Pool{
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", "localhost:6379")
			if err != nil {
				return nil, err
			}
			return c, err
		},
	}
	defer pool.Close()

	quota := limiter.Quota{Limit: 3, Within: 1 * time.Minute}
	limiter := limiter.NewLimiter(quota, redigo.NewRedigoAdaptor(&pool))

	handler := http.FileServer(http.Dir("."))
	port := ":8080"
	log.Println("Going to listen on " + port)
	log.Fatal(http.ListenAndServe(port, limiter.Handle(handler)))
}
