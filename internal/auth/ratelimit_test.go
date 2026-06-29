package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimitTokenBucket(t *testing.T) {
	rl := &rateLimiter{
		buckets:  make(map[string]*bucket),
		capacity: 3,
		refill:   3.0 / 60.0,
		now:      time.Now,
	}

	for i := 0; i < 3; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("call %d should be allowed", i+1)
		}
	}
	if rl.allow("1.2.3.4") {
		t.Fatal("4th call should be denied")
	}
	if !rl.allow("9.9.9.9") {
		t.Fatal("other IP should not share bucket")
	}
}

func TestRateLimitRefillsOverTime(t *testing.T) {
	base := time.Now()
	rl := &rateLimiter{
		buckets:  make(map[string]*bucket),
		capacity: 1,
		refill:   1.0,
		now:      func() time.Time { return base },
	}
	if !rl.allow("ip") {
		t.Fatal("first allowed")
	}
	if rl.allow("ip") {
		t.Fatal("second denied")
	}
	rl.now = func() time.Time { return base.Add(2 * time.Second) }
	if !rl.allow("ip") {
		t.Fatal("after refill allowed")
	}
}

func TestRateLimitMiddleware429(t *testing.T) {
	mw := NewRateLimitMiddleware(1, time.Minute)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.RemoteAddr = "1.1.1.1:1234"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w1.Code)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.RemoteAddr = "1.1.1.1:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w2.Code)
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func TestKeyedRateLimitPerUser(t *testing.T) {

	mw := NewKeyedRateLimitMiddleware(1, time.Minute, func(r *http.Request) string {
		return r.Header.Get("X-User")
	})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	call := func(user string) int {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = "1.1.1.1:1"
		if user != "" {
			r.Header.Set("X-User", user)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}

	if got := call("alice"); got != http.StatusOK {
		t.Fatalf("alice 1st: want 200, got %d", got)
	}
	if got := call("alice"); got != http.StatusTooManyRequests {
		t.Fatalf("alice 2nd: want 429, got %d", got)
	}

	if got := call("bob"); got != http.StatusOK {
		t.Fatalf("bob 1st: want 200, got %d", got)
	}
}

func TestKeyedRateLimitFallsBackToIP(t *testing.T) {

	mw := NewKeyedRateLimitMiddleware(1, time.Minute, func(*http.Request) string { return "" })
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	call := func() int {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = "2.2.2.2:9"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	if got := call(); got != http.StatusOK {
		t.Fatalf("1st: want 200, got %d", got)
	}
	if got := call(); got != http.StatusTooManyRequests {
		t.Fatalf("2nd (same IP): want 429, got %d", got)
	}
}

func TestClientIPNoPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "no-port"
	if got := clientIP(r); got != "no-port" {
		t.Fatalf("expected fallback to raw addr, got %q", got)
	}
}
