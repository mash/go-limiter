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
	"github.com/mash/go-limiter/redigostore"
)

func main() {
	pool := redis.Pool{
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", "localhost:6379")
		},
	}
	defer pool.Close()

	quota := limiter.Quota{Limit: 3, Within: 1 * time.Minute}

	// try:
	// curl -v "http://localhost:8080/" -H "X-User-Id: 1"
	l := limiter.New(quota, redigostore.New(&pool), limiter.Key, limiter.HeaderIdentifier("X-User-Id"), limiter.DefaultErrorHandler)

	handler := http.FileServer(http.Dir("."))
	port := ":8080"
	log.Println("Going to listen on " + port)
	log.Fatal(http.ListenAndServe(port, l.Handle(handler)))
}
```
