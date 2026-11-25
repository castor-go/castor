package middleware

import (
	"net/http"
	"strconv"
	"time"
)

const (
	errCapacityExceeded = "Server capacity exceeded."
	errTimedOut         = "Timed out while waiting for a pending request to complete."
	errContextCanceled  = "Context was canceled."
)

const defaultBacklogTimeout = 60 * time.Second

// ThrottleOpts configures ThrottleWithOpts.
type ThrottleOpts struct {
	// RetryAfterFn, if set, enables the Retry-After header on rejected requests.
	RetryAfterFn   func(ctxDone bool) time.Duration
	Limit          int
	BacklogLimit   int
	BacklogTimeout time.Duration
	StatusCode     int
}

// Throttle limits the number of in-flight requests.
func Throttle(limit int) func(http.Handler) http.Handler {
	return ThrottleWithOpts(ThrottleOpts{Limit: limit, BacklogTimeout: defaultBacklogTimeout})
}

// ThrottleBacklog limits in-flight requests and allows a finite backlog.
func ThrottleBacklog(limit, backlogLimit int, backlogTimeout time.Duration) func(http.Handler) http.Handler {
	return ThrottleWithOpts(ThrottleOpts{
		Limit:          limit,
		BacklogLimit:   backlogLimit,
		BacklogTimeout: backlogTimeout,
	})
}

// ThrottleWithOpts limits in-flight requests using opts.
func ThrottleWithOpts(opts ThrottleOpts) func(http.Handler) http.Handler {
	if opts.Limit < 1 {
		panic("middleware: Throttle expects limit > 0")
	}
	if opts.BacklogLimit < 0 {
		panic("middleware: Throttle expects backlogLimit >= 0")
	}

	statusCode := opts.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusTooManyRequests
	}

	throttler := &throttler{
		tokens:         make(chan struct{}, opts.Limit),
		backlogTokens:  make(chan struct{}, opts.Limit+opts.BacklogLimit),
		backlogTimeout: opts.BacklogTimeout,
		statusCode:     statusCode,
		retryAfterFn:   opts.RetryAfterFn,
	}
	for i := 0; i < opts.Limit+opts.BacklogLimit; i++ {
		if i < opts.Limit {
			throttler.tokens <- struct{}{}
		}
		throttler.backlogTokens <- struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			select {
			case <-ctx.Done():
				throttler.setRetryAfterHeaderIfNeeded(w, true)
				http.Error(w, errContextCanceled, throttler.statusCode)
				return

			case backlogToken := <-throttler.backlogTokens:
				defer func() { throttler.backlogTokens <- backlogToken }()

				select {
				case token := <-throttler.tokens:
					defer func() { throttler.tokens <- token }()
					next.ServeHTTP(w, r)
					return
				default:
				}

				timer := time.NewTimer(throttler.backlogTimeout)
				select {
				case <-timer.C:
					throttler.setRetryAfterHeaderIfNeeded(w, false)
					http.Error(w, errTimedOut, throttler.statusCode)

				case <-ctx.Done():
					timer.Stop()
					throttler.setRetryAfterHeaderIfNeeded(w, true)
					http.Error(w, errContextCanceled, throttler.statusCode)

				case token := <-throttler.tokens:
					defer func() {
						timer.Stop()
						throttler.tokens <- token
					}()
					next.ServeHTTP(w, r)
				}

			default:
				throttler.setRetryAfterHeaderIfNeeded(w, false)
				http.Error(w, errCapacityExceeded, throttler.statusCode)
			}
		})
	}
}

type throttler struct {
	tokens         chan struct{}
	backlogTokens  chan struct{}
	retryAfterFn   func(ctxDone bool) time.Duration
	backlogTimeout time.Duration
	statusCode     int
}

func (t *throttler) setRetryAfterHeaderIfNeeded(w http.ResponseWriter, ctxDone bool) {
	if t.retryAfterFn == nil {
		return
	}

	delay := t.retryAfterFn(ctxDone)
	seconds := delay / time.Second
	if delay < 0 {
		seconds = 0
	} else if delay%time.Second != 0 {
		seconds++
	}
	w.Header().Set("Retry-After", strconv.FormatInt(int64(seconds), 10))
}
