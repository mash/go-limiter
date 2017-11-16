package limiter

import (
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/mash/go-limiter/redigostore"
	"github.com/soh335/go-test-redisserver"
)

// will be nil if we're testing on wercker
func MustStartRedisTestServer() *redistest.Server {
	if os.Getenv("WERCKER_REDIS_PORT") != "" && os.Getenv("WERCKER_REDIS_HOST") != "" {
		os.Setenv("REDIS_ADDRESS", os.Getenv("WERCKER_REDIS_HOST")+":"+os.Getenv("WERCKER_REDIS_PORT"))
	} else {
		server, err := redistest.NewServer(true, nil)
		if err != nil {
			panic(err)
		}
		os.Setenv("REDIS_NETWORK", "unix")
		os.Setenv("REDIS_ADDRESS", server.Config["unixsocket"])
		return server
	}
	return nil
}

func redispool() redis.Pool {
	return redis.Pool{
		Dial: func() (redis.Conn, error) {
			redisNetwork := os.Getenv("REDIS_NETWORK")
			if redisNetwork == "" {
				redisNetwork = "tcp"
			}
			redisAddress := os.Getenv("REDIS_ADDRESS")
			if redisAddress == "" {
				redisAddress = "localhost:6379"
			}
			conn, err := redis.Dial(redisNetwork, redisAddress)
			logger := log.New(os.Stdout, "", log.Lmicroseconds|log.Lshortfile)

			return redis.NewLoggingConn(conn, logger, "[Redis]"), err
		},
	}
}

type incrementer struct {
	count int
}

func (i *incrementer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte(strconv.Itoa(i.count)))
	i.count = i.count + 1
}

func serveHTTP(handler http.Handler) (*httptest.ResponseRecorder, error) {
	recorder := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		return nil, err
	}
	req.RemoteAddr = "127.0.0.1:9999" // to satisfy IPIdentify

	handler.ServeHTTP(recorder, req)
	return recorder, nil
}

func TestLimiter(t *testing.T) {
	redisserver := MustStartRedisTestServer()
	if redisserver != nil {
		defer redisserver.Stop()
	}
	pool := redispool()
	defer pool.Close()

	quota := Quota{Limit: 3, Within: 1 * time.Second}
	l := New(quota, redigostore.New(&pool), Key, HeaderIdentifier("X-USER-ID"), DefaultErrorHandler)
	i := incrementer{count: 0}
	handler := l.Handle(&i)

	tests := []struct {
		UserId                              string
		Code                                int
		Description, Body, Limit, Remaining string
	}{
		{
			Description: "1st request",
			UserId:      "1",
			Code:        200,
			Body:        "0",
			Limit:       "3",
			Remaining:   "2",
		},
		{
			Description: "2nd request",
			UserId:      "1",
			Code:        200,
			Body:        "1",
			Limit:       "3",
			Remaining:   "1",
		},
		{
			Description: "3rd request",
			UserId:      "1",
			Code:        200,
			Body:        "2",
			Limit:       "3",
			Remaining:   "0",
		},
		{
			Description: "4th request gets 429",
			UserId:      "1",
			Code:        429,
			Body:        "Too Many Requests\n", // incrementer doesn't count up
			Limit:       "3",
			Remaining:   "0",
		},
		{
			Description: "1st request from user2",
			UserId:      "2",
			Code:        200,
			Body:        "3",
			Limit:       "3",
			Remaining:   "2",
		}}

	for _, test := range tests {
		req, _ := http.NewRequest("GET", "/", nil)
		req.Header.Set("X-USER-ID", test.UserId)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		t.Logf("--- %s", test.Description)
		if recorder.Code != test.Code {
			t.Errorf("Expected Code: %d but got: %d, recorder: %#v", test.Code, recorder.Code, recorder)
		}
		body := recorder.Body.String()
		if body != test.Body {
			t.Errorf("Expected Body: %s but got: %s, recorder: %#v", test.Body, body, recorder)
		}
		if recorder.HeaderMap.Get("X-Rate-Limit-Limit") != test.Limit {
			t.Errorf("Expected X-Rate-Limit-Limit: %s but got: %s, recorder: %#v", test.Limit, recorder.HeaderMap.Get("X-Rate-Limit-Limit"), recorder)
		}
		if recorder.HeaderMap.Get("X-Rate-Limit-Remaining") != test.Remaining {
			t.Errorf("Expected X-Rate-Limit-Remaining: %s but got: %s, recorder: %#v", test.Remaining, recorder.HeaderMap.Get("X-Rate-Limit-Remaining"), recorder)
		}
	}
}
