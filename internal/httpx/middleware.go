package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

type ctxKey struct{ name string }

var (
	requestIDKey = ctxKey{"request-id"}
	loggerKey    = ctxKey{"logger"}
)

func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

func Logger(ctx context.Context) *slog.Logger {
	if v, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return v
	}
	return slog.Default()
}

func WithRequestID(base *slog.Logger) func(http.Handler) http.Handler {
	if base == nil {
		base = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = newID()
			}
			w.Header().Set("X-Request-Id", id)

			lg := base.With(
				slog.String("request_id", id),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			ctx = context.WithValue(ctx, loggerKey, lg)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		Logger(r.Context()).Info(
			"http",
			slog.Int("status", sw.status),
			slog.Int("bytes", sw.bytes),
			slog.Duration("dur", time.Since(start)),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if s.wrote {
		return
	}
	s.status = code
	s.wrote = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if !s.wrote {
		s.WriteHeader(http.StatusOK)
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func StatusOf(w http.ResponseWriter) int {
	if s, ok := w.(*statusWriter); ok {
		return s.status
	}
	return http.StatusOK
}

func StatusClass(code int) string {
	return strconv.Itoa(code/100) + "xx"
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
