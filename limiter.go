package limiter

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	// The header name to retrieve an IP address under a proxy
	forwardedForHeader = "X-FORWARDED-FOR"
)

var (
	KeyPrefix = "limiter"
)

type Quota struct {
	Limit  int64
	Within time.Duration
}

func (q Quota) ResetsAt(now time.Time) time.Time {
	seconds := now.Unix()
	within := int64(q.Within.Seconds())
	return time.Unix((seconds/within+1)*within, 0)
}

type Keyer func(now time.Time, slot int64, identifier string) string

type Identifier func(req *http.Request) (string, error)

type ErrorHandler func(w http.ResponseWriter, req *http.Request, result Result, err error)

type Result struct {
	Denied    bool
	ResetsAt  time.Time
	Remaining int64
	Id        string
	Counter   int64
}

type Limiter struct {
	quota        Quota
	store        Store
	Keyer        Keyer
	Identifier   Identifier
	ErrorHandler ErrorHandler
}

func New(q Quota, store Store, keyer Keyer, identifier Identifier, errorHandler ErrorHandler) Limiter {
	return Limiter{
		quota:        q,
		store:        store,
		Keyer:        keyer,
		Identifier:   identifier,
		ErrorHandler: errorHandler,
	}
}

func (l Limiter) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		id, err := l.Identifier(req)
		if err != nil {
			l.ErrorHandler(w, req, Result{}, err)
			return
		}
		if id == "" {
			// empty ids have no limits
			next.ServeHTTP(w, req)
			return
		}

		now := time.Now()
		slot := now.Unix() / int64(l.quota.Within.Seconds())
		key := l.Keyer(now, slot, id)
		count, err := l.store.Get(key, l.quota.Within)
		if err != nil {
			l.ErrorHandler(w, req, Result{}, err)
			return
		}
		result := Result{
			Denied:    count > l.quota.Limit,
			ResetsAt:  l.quota.ResetsAt(now),
			Remaining: max(l.quota.Limit-count, 0),
			Id:        id,
			Counter:   count,
		}
		l.SetRateLimitHeaders(w, result)
		if result.Denied {
			l.ErrorHandler(w, req, result, nil)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func (l Limiter) SetRateLimitHeaders(w http.ResponseWriter, result Result) {
	headers := w.Header()
	headers.Set("X-Rate-Limit-Limit", strconv.FormatInt(l.quota.Limit, 10))
	headers.Set("X-Rate-Limit-Reset", strconv.FormatInt(result.ResetsAt.Unix(), 10))
	headers.Set("X-Rate-Limit-Remaining", strconv.FormatInt(result.Remaining, 10))
}

// Key is a limiter.Keyer
func Key(now time.Time, slot int64, identifier string) string {
	return fmt.Sprintf("%s-%d-%s", KeyPrefix, slot, identifier)
}

// DefaultErrorHandler is a simple error handler that responds with status code 429 when exceeding limits and 500 on any error.
func DefaultErrorHandler(w http.ResponseWriter, req *http.Request, result Result, err error) {
	if err != nil {
		log.Println(err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	} else if result.Denied {
		http.Error(w, "Too Many Requests", 429)
	}
}

func IPIdentifier(req *http.Request) (string, error) {
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

func HeaderIdentifier(name string) Identifier {
	return func(req *http.Request) (string, error) {
		return req.Header.Get(name), nil
	}
}
