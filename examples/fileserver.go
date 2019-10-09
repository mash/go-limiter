package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/mash/go-limiter"
	"github.com/mash/go-limiter/redigostore"
)

func main() {
	pool := redis.Pool{
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", "localhost:6379")
		},
	}
	defer pool.Close()

	// fail early in case you havent started Redis yet
	conn := pool.Get()
	if err := conn.Err(); err != nil {
		log.Fatalf("%s", err)
	}
	defer conn.Close()

	// try:
	// curl -v "http://localhost:8080/" -H "X-User-Id: 1"

	quota := limiter.Quota{Limit: 3, Within: 1 * time.Minute}
	l := limiter.New(quota,
		redigostore.New(&pool),
		limiter.Key,
		limiter.HeaderIdentifier("X-User-Id"))
	// or limiter.Default(quota, redigostore.New(&pool))

	handler := http.FileServer(http.Dir("."))
	port := ":8080"
	log.Println("Going to listen on " + port)
	log.Fatal(http.ListenAndServe(port, l.Handle(handler, limiter.DefaultErrorHandler)))
}
