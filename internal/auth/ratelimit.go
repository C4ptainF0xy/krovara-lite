package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var trustProxy bool

func SetTrustProxy(v bool) { trustProxy = v }

type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity float64
	refill   float64
	now      func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func NewRateLimitMiddleware(capacity int, window time.Duration) func(http.Handler) http.Handler {
	rl := &rateLimiter{
		buckets:  make(map[string]*bucket),
		capacity: float64(capacity),
		refill:   float64(capacity) / window.Seconds(),
		now:      time.Now,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.allow(clientIP(r)) {
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func NewKeyedRateLimitMiddleware(capacity int, window time.Duration, key func(*http.Request) string) func(http.Handler) http.Handler {
	rl := &rateLimiter{
		buckets:  make(map[string]*bucket),
		capacity: float64(capacity),
		refill:   float64(capacity) / window.Seconds(),
		now:      time.Now,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k := key(r)
			if k == "" {
				k = "ip:" + clientIP(r)
			}
			if !rl.allow(k) {
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	b, ok := rl.buckets[key]
	if !ok {
		rl.buckets[key] = &bucket{tokens: rl.capacity - 1, last: now}
		return true
	}

	elapsed := now.Sub(b.last).Seconds()
	b.tokens = min(rl.capacity, b.tokens+elapsed*rl.refill)
	b.last = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func clientIP(r *http.Request) string {
	if trustProxy {

		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			return xr
		}

		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
