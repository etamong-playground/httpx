// Package ratelimit provides an in-memory per-IP token-bucket rate limiter
// that integrates with [github.com/etamong-playground/httperr] for 429
// responses. It keys visitors by the IP returned by
// [github.com/etamong-playground/httperr/ip.ClientIP].
//
// State is held in a single in-process map. A distributed store (e.g. a
// CNPG table or Valkey cluster) would slot in here, replacing the visitor
// map and getVisitor/cleanup methods, once multi-replica rate-limiting is
// needed.
package ratelimit

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/etamong-playground/httperr"
	"github.com/etamong-playground/httperr/ip"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter is an in-memory per-IP rate limiter backed by token buckets.
// Construct one with [New] and defer [RateLimiter.Stop] to shut down the
// background cleanup goroutine.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	r        rate.Limit
	burst    int
	resp     *httperr.Responder
	stopOnce sync.Once
	stopCh   chan struct{}
}

// New creates a rate limiter that allows r requests per second with the given
// burst size. resp is used to emit the 429 response; pass the same
// [httperr.Responder] shared across the service. A background goroutine
// evicting visitors idle for more than 5 minutes is started immediately.
func New(r rate.Limit, burst int, resp *httperr.Responder) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		r:        r,
		burst:    burst,
		resp:     resp,
		stopCh:   make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Stop terminates the background cleanup goroutine. It is safe to call Stop
// multiple times.
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() { close(rl.stopCh) })
}

func (rl *RateLimiter) getVisitor(clientIP string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[clientIP]
	if !exists {
		limiter := rate.NewLimiter(rl.r, rl.burst)
		rl.visitors[clientIP] = &visitor{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}
	v.lastSeen = time.Now()
	return v.limiter
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			for clientIP, v := range rl.visitors {
				if time.Since(v.lastSeen) > 5*time.Minute {
					delete(rl.visitors, clientIP)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

// Handler returns an HTTP middleware that rate-limits requests per client IP.
// When the limit is exceeded it sets Retry-After and X-RateLimit-Limit headers
// and delegates the 429 response to the injected [httperr.Responder].
func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	limitHeader := strconv.Itoa(rl.burst)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := ip.ClientIP(r)
		limiter := rl.getVisitor(clientIP)
		rsv := limiter.Reserve()
		if !rsv.OK() {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("X-RateLimit-Limit", limitHeader)
			rl.resp.Fail(w, r, http.StatusTooManyRequests, "too many requests, please try again later", nil)
			return
		}
		delay := rsv.Delay()
		if delay > 0 {
			rsv.Cancel()
			retryAfter := int(math.Ceil(delay.Seconds()))
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.Header().Set("X-RateLimit-Limit", limitHeader)
			rl.resp.Fail(w, r, http.StatusTooManyRequests, "too many requests, please try again later", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
