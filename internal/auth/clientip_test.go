package auth

import (
	"net/http"
	"testing"
)

func TestClientIP_TrustProxy(t *testing.T) {
	t.Cleanup(func() { SetTrustProxy(false) })

	req := func(remote, realIP, xff string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		if realIP != "" {
			r.Header.Set("X-Real-IP", realIP)
		}
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}

	SetTrustProxy(false)
	if got := clientIP(req("203.0.113.9:5555", "1.2.3.4", "9.9.9.9")); got != "203.0.113.9" {
		t.Fatalf("untrusted: want peer host, got %q", got)
	}

	SetTrustProxy(true)
	if got := clientIP(req("127.0.0.1:443", "198.51.100.7", "9.9.9.9, 127.0.0.1")); got != "198.51.100.7" {
		t.Fatalf("trusted: want X-Real-IP, got %q", got)
	}

	if got := clientIP(req("127.0.0.1:443", "", "198.51.100.8, 10.0.0.1")); got != "198.51.100.8" {
		t.Fatalf("trusted XFF: want leftmost, got %q", got)
	}

	if got := clientIP(req("203.0.113.9:5555", "", "")); got != "203.0.113.9" {
		t.Fatalf("trusted fallback: want peer host, got %q", got)
	}
}
