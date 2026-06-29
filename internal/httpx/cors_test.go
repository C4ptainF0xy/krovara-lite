package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/krovara/krovara/internal/httpx"
)

func TestCORS(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mw := httpx.CORS([]string{"http://tauri.localhost", "tauri://localhost"})
	h := mw(ok)

	t.Run("allowed origin reflected with credentials", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		r.Header.Set("Origin", "http://tauri.localhost")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://tauri.localhost" {
			t.Fatalf("allow-origin = %q, want the reflected origin", got)
		}
		if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Fatal("missing allow-credentials")
		}
	})

	t.Run("preflight from allowed origin returns 204", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodOptions, "/api/auth/login", nil)
		r.Header.Set("Origin", "tauri://localhost")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("preflight status = %d, want 204", w.Code)
		}
	})

	t.Run("unknown origin gets no CORS headers", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		r.Header.Set("Origin", "https://evil.example")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("allow-origin leaked to a non-allowlisted origin")
		}
	})

	t.Run("unknown origin preflight is forbidden", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodOptions, "/api/me", nil)
		r.Header.Set("Origin", "https://evil.example")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})

	t.Run("empty allowlist is a no-op", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		r.Header.Set("Origin", "http://tauri.localhost")
		w := httptest.NewRecorder()
		httpx.CORS(nil)(ok).ServeHTTP(w, r)
		if w.Header().Get("Access-Control-Allow-Origin") != "" || w.Code != http.StatusOK {
			t.Fatal("empty allowlist should not add CORS headers")
		}
	})
}
