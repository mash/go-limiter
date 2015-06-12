package limiter

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/mash/go-limiter/adaptor/redigo"
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
			return redis.Dial(redisNetwork, redisAddress)
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
	limiter := NewLimiter(quota, redigo.NewRedigoAdaptor(pool))
	i := incrementer{count: 0}
	handler := limiter.Handle(&i)

	expectedResults := []struct {
		Code      int
		Body      string
		Limit     string
		Remaining string
	}{
		{
			Code:      200,
			Body:      "0",
			Limit:     "3",
			Remaining: "2",
		},
		{
			Code:      200,
			Body:      "1",
			Limit:     "3",
			Remaining: "1",
		},
		{
			Code:      200,
			Body:      "2",
			Limit:     "3",
			Remaining: "0",
		},
		{
			Code:      429,
			Body:      "Too Many Requests\n",
			Limit:     "3",
			Remaining: "0",
		},
	}

	for _, expectedResult := range expectedResults {
		recorder, err := serveHTTP(handler)
		if err != nil {
			t.Fatalf(err.Error())
		}
		if recorder.Code != expectedResult.Code {
			t.Errorf("Expected %d but got: %d, recorder: %#v", expectedResult.Code, recorder.Code, recorder)
		}
		body := recorder.Body.String()
		if body != expectedResult.Body {
			t.Errorf("Expected %s but got: %s, recorder: %#v", expectedResult.Body, body, recorder)
		}
		if recorder.HeaderMap.Get("X-Rate-Limit-Limit") != expectedResult.Limit {
			t.Errorf("Expected %s but got: %s, recorder: %#v", expectedResult.Limit, recorder.HeaderMap.Get("X-Rate-Limit-Limit"), recorder)
		}
		if recorder.HeaderMap.Get("X-Rate-Limit-Remaining") != expectedResult.Remaining {
			t.Errorf("Expected %s but got: %s, recorder: %#v", expectedResult.Remaining, recorder.HeaderMap.Get("X-Rate-Limit-Remaining"), recorder)
		}
	}
}
