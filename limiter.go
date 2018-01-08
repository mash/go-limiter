package limiter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	// The header name to retrieve an IP address under a proxy
	forwardedForHeader = "X-FORWARDED-FOR"
)

type contextkey int

const (
	ResultContextKey contextkey = iota
)

var (
	KeyPrefix = "limiter"
	Error     = errors.New("too many requests")
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

type ErrorHandler func(w http.ResponseWriter, req *http.Request, err error)

type Result struct {
	Denied    bool
	ResetsAt  time.Time
	Remaining int64
	Id        string
	Counter   int64
}

type Limiter struct {
	quota      Quota
	store      Store
	Keyer      Keyer
	Identifier Identifier
}

func Default(q Quota, store Store) Limiter {
	return New(q, store, Key, HeaderIdentifier("Authorization"))
}

func New(q Quota, store Store, keyer Keyer, identifier Identifier) Limiter {
	return Limiter{
		quota:      q,
		store:      store,
		Keyer:      keyer,
		Identifier: identifier,
	}
}

func (l Limiter) Check(req *http.Request) (Result, error) {
	id, err := l.Identifier(req)
	if err != nil {
		return Result{}, err
	}
	if id == "" {
		// empty ids have no limits
		return Result{}, nil
	}

	now := time.Now()
	slot := now.Unix() / int64(l.quota.Within.Seconds())
	key := l.Keyer(now, slot, id)
	count, err := l.store.Get(key, l.quota.Within)
	if err != nil {
		return Result{
			Id: id,
		}, err
	}
	result := Result{
		Denied:    count > l.quota.Limit,
		ResetsAt:  l.quota.ResetsAt(now),
		Remaining: max(l.quota.Limit-count, 0),
		Id:        id,
		Counter:   count,
	}
	if count > l.quota.Limit {
		return result, Error
	}
	return result, nil
}

func (l Limiter) Handle(next http.Handler, errorHandler ErrorHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		result, err := l.Check(req)

		ctx := req.Context()
		ctx = context.WithValue(ctx, ResultContextKey, result)
		req = req.WithContext(ctx)

		SetHeader(l.quota)(w, req)

		if err != nil {
			errorHandler(w, req, err)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func SetHeader(quota Quota) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		result, ok := Value(req)
		if ok && result.Id != "" {
			headers := w.Header()
			headers.Set("X-Rate-Limit-Limit", strconv.FormatInt(quota.Limit, 10))
			headers.Set("X-Rate-Limit-Reset", strconv.FormatInt(result.ResetsAt.Unix(), 10))
			headers.Set("X-Rate-Limit-Remaining", strconv.FormatInt(result.Remaining, 10))
		}
	}
}

func SetHeaderMiddleware(quota Quota) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)
			SetHeader(quota)(w, req)
		})
	}
}

// Shortcut
func Value(req *http.Request) (Result, bool) {
	ctx := req.Context()
	r, ok := ctx.Value(ResultContextKey).(Result)
	return r, ok
}

// Key is a limiter.Keyer
func Key(now time.Time, slot int64, identifier string) string {
	return fmt.Sprintf("%s-%d-%s", KeyPrefix, slot, identifier)
}

// DefaultErrorHandler is a simple error handler that responds with status code 429 when exceeding limits and 500 on any error.
func DefaultErrorHandler(w http.ResponseWriter, req *http.Request, err error) {
	if err == Error {
		http.Error(w, "Too Many Requests", 429)
	} else {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
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

func ContextIdentifier(key interface{}) Identifier {
	return func(req *http.Request) (string, error) {
		ctxt := req.Context()
		val := ctxt.Value(key)
		if val == nil {
			return "", nil
		}
		if s, ok := val.(string); ok {
			return s, nil
		}
		return "", fmt.Errorf("type assertion failed from: %#v", ctxt.Value(key))
	}
}

func HeaderIdentifier(name string) Identifier {
	return func(req *http.Request) (string, error) {
		return req.Header.Get(name), nil
	}
}
