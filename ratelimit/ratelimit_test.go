package ratelimit_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/etamong-playground/httperr"
	"github.com/etamong-playground/httperr/ratelimit"
	"golang.org/x/time/rate"
)

// testResponder returns a minimal *httperr.Responder that discards log output.
func testResponder() *httperr.Responder {
	return &httperr.Responder{
		Log: httperr.NewLogger(io.Discard),
		App: "test",
	}
}

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := ratelimit.New(rate.Limit(10), 10, testResponder())
	defer rl.Stop()

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for range 5 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	}
}

func TestRateLimiter_BlocksExcessRequests(t *testing.T) {
	rl := ratelimit.New(rate.Limit(1), 2, testResponder())
	defer rl.Stop()

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust burst
	for range 2 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 during burst, got %d", rec.Code)
		}
	}

	// Next request should be rate limited
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header to be set")
	}
}

func TestRateLimiter_DifferentIPsIndependent(t *testing.T) {
	rl := ratelimit.New(rate.Limit(1), 1, testResponder())
	defer rl.Stop()

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IP A exhausts its limit
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// IP A is now blocked
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for exhausted IP A, got %d", rec.Code)
	}

	// IP B should still be allowed
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "5.6.7.8:5678"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for fresh IP B, got %d", rec.Code)
	}
}

func TestRateLimiter_RetryAfterHeader(t *testing.T) {
	rl := ratelimit.New(rate.Limit(0.5), 1, testResponder())
	defer rl.Stop()

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust burst
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)

	// Rate limited — Retry-After should be >= 1
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("expected Retry-After header")
	}
	val, err := strconv.Atoi(retryAfter)
	if err != nil {
		t.Fatalf("Retry-After %q is not an integer: %v", retryAfter, err)
	}
	if val < 1 {
		t.Fatalf("Retry-After = %d, want >= 1", val)
	}
}

func TestRateLimiter_StopIdempotent(t *testing.T) {
	rl := ratelimit.New(rate.Limit(1), 1, testResponder())
	rl.Stop()
	rl.Stop() // must not panic
}
