package limiter

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// The header name to retrieve an IP address under a proxy
	forwardedForHeader = "X-FORWARDED-FOR"
)

type Quota struct {
	Limit  uint64
	Within time.Duration
}

func (q Quota) ResetUnix(now time.Time) int64 {
	seconds := now.Unix()
	within := int64(q.Within.Seconds())
	return (seconds/within + 1) * within
}

type Result struct {
	Denied     bool
	ResetUnix  int64
	Remaining  uint64
	Identifier string
	Counter    uint64
}

type RedisClient interface {
	Get(key string) (uint64, error)
	Incrx(key string, seconds int) error
}

type Limiter struct {
	quota                   Quota
	redis                   RedisClient
	keyPrefix, keyDelimiter string
	Identify                func(req *http.Request) (string, error)
	ErrorHandler            func(w http.ResponseWriter, req *http.Request, err error)
	DeniedHandler           func(w http.ResponseWriter, req *http.Request, result Result)
}

func NewLimiter(q Quota, redis RedisClient) Limiter {
	return Limiter{
		quota:         q,
		redis:         redis,
		keyPrefix:     "limiter",
		keyDelimiter:  "-",
		Identify:      IPIdentify,
		ErrorHandler:  DefaultErrorHandler,
		DeniedHandler: DefaultDeniedHandler,
	}
}

func (l Limiter) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		identifier, err := l.Identify(req)
		if err != nil {
			l.ErrorHandler(w, req, err)
			return
		}
		if identifier != "" {
			result, err := l.CheckLimit(identifier)
			if err != nil {
				l.ErrorHandler(w, req, err)
				return
			}
			l.SetRateLimitHeaders(w, *result)
			if result.Denied {
				l.DeniedHandler(w, req, *result)
				return
			}
		}
		next.ServeHTTP(w, req)
	})
}

func (l Limiter) CheckLimit(identifier string) (*Result, error) {
	now := time.Now()
	key := l.Key(now, identifier)

	counter, err := l.redis.Get(key)
	if err != nil {
		return nil, err
	}

	reset := l.quota.ResetUnix(now)

	if counter < l.quota.Limit {
		err = l.redis.Incrx(key, int(l.quota.Within.Seconds()))
		if err != nil {
			return nil, err
		}
		return &Result{
			Denied:     false,
			ResetUnix:  reset,
			Remaining:  l.quota.Limit - counter - 1,
			Identifier: identifier,
			Counter:    counter,
		}, nil
	}
	return &Result{
		Denied:     true,
		ResetUnix:  reset,
		Remaining:  0,
		Identifier: identifier,
		Counter:    counter,
	}, nil
}

func (l Limiter) Key(now time.Time, identifier string) string {
	return strings.Join([]string{
		l.keyPrefix,
		strconv.FormatInt(now.Unix()/int64(l.quota.Within.Seconds()), 10),
		identifier,
	}, l.keyDelimiter)
}

func (l Limiter) SetRateLimitHeaders(w http.ResponseWriter, result Result) {
	headers := w.Header()
	headers.Set("X-Rate-Limit-Limit", strconv.FormatUint(l.quota.Limit, 10))
	headers.Set("X-Rate-Limit-Reset", strconv.FormatInt(result.ResetUnix, 10))
	headers.Set("X-Rate-Limit-Remaining", strconv.FormatUint(result.Remaining, 10))
}

func DefaultErrorHandler(w http.ResponseWriter, req *http.Request, err error) {
	log.Println(err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func DefaultDeniedHandler(w http.ResponseWriter, req *http.Request, result Result) {
	http.Error(w, "Too Many Requests", 429)
}

func IPIdentify(req *http.Request) (string, error) {
	if forwardedFor := req.Header.Get(forwardedForHeader); forwardedFor != "" {
		if ipParsed := net.ParseIP(forwardedFor); ipParsed != nil {
			return ipParsed.String(), nil
		}
	}
	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return "", err
	}
	return ip, nil
}
