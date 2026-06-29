package httpx_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/krovara/krovara/internal/httpx"
)

func TestWithRequestID_GeneratesAndExposes(t *testing.T) {
	var seen string
	h := httpx.WithRequestID(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestID(r.Context())
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.NotEmpty(t, seen)
	require.Equal(t, seen, rr.Header().Get("X-Request-Id"))
}

func TestWithRequestID_HonorsIncomingHeader(t *testing.T) {
	h := httpx.WithRequestID(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		require.Equal(t, "client-supplied", httpx.RequestID(r.Context()))
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "client-supplied")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, "client-supplied", rr.Header().Get("X-Request-Id"))
}

func TestAccessLog_EmitsRequestFields(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))

	r := chi.NewRouter()
	r.Use(httpx.WithRequestID(base))
	r.Use(httpx.AccessLog)
	r.Get("/x", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "hi")
	})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	require.Equal(t, http.StatusTeapot, rr.Code)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.NotEmpty(t, lines)
	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &entry))
	require.Equal(t, "GET", entry["method"])
	require.Equal(t, "/x", entry["path"])
	require.EqualValues(t, http.StatusTeapot, entry["status"])
	require.NotEmpty(t, entry["request_id"])
}

func TestMetrics_RecordsAndExposes(t *testing.T) {
	r := chi.NewRouter()
	r.Use(httpx.Metrics(func(req *http.Request) string {
		return chi.RouteContext(req.Context()).RoutePattern()
	}))
	r.Get("/api/spaces/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/spaces/abc", nil))
	require.Equal(t, http.StatusOK, rr.Code)

	scrape := httptest.NewRecorder()
	httpx.MetricsHandler().ServeHTTP(scrape, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := scrape.Body.String()
	require.Contains(t, body, "krovara_http_requests_total")

	require.Contains(t, body, `route="/api/spaces/{id}"`)
	require.NotContains(t, body, `route="/api/spaces/abc"`, "raw path leaked into metric label")
}
