package httpx

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "krovara_http_request_duration_seconds",
			Help:    "Request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "status_class", "route"},
	)
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "krovara_http_requests_total",
			Help: "Count of HTTP requests served.",
		},
		[]string{"method", "status_class", "route"},
	)
)

func init() {
	prometheus.MustRegister(httpDuration, httpRequests)
}

func Metrics(routeFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw, ok := w.(*statusWriter)
			if !ok {
				sw = &statusWriter{ResponseWriter: w, status: http.StatusOK}
			}
			next.ServeHTTP(sw, r)
			route := "unknown"
			if routeFn != nil {
				if rt := routeFn(r); rt != "" {
					route = rt
				}
			}
			labels := prometheus.Labels{
				"method":       r.Method,
				"status_class": StatusClass(sw.status),
				"route":        route,
			}
			httpDuration.With(labels).Observe(time.Since(start).Seconds())
			httpRequests.With(labels).Inc()
		})
	}
}

func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
