Request limiter/throttler using Redis in golang
===============================================

## Description

A HTTP server middleware for limiting/throttling requests using Redis INCR.

## Usage

``` golang
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/mash/go-limiter"
	"github.com/mash/go-limiter/redigo"
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
	client := pool.Get()
	defer client.Close()

	quota := limiter.Quota{Limit: 3, Within: 1 * time.Minute}
	limiter := limiter.NewLimiter(quota, redigo.NewRedigoAdaptor(client))

	handler := http.FileServer(http.Dir("."))
	port := ":8080"
	log.Println("Going to listen on " + port)
	log.Fatal(http.ListenAndServe(port, limiter.Handle(handler)))
}
```
