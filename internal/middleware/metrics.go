package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Nuriklan/digital-commerce/pkg/metrics"
)

// Metrics returns middleware for the automatic collection of HTTP metrics (RPS, latency, status codes).
func Metrics(m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// We exclude the metrics and health-check endpoints themselves from business metric collection.
			if r.URL.Path == "/metrics" || r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			wrapped := newResponseWriterWrapper(w)

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start).Seconds()

			// High Cardinality Protection:
			// Go 1.22+ r.Pattern contains the route pattern (например, "GET /orders/{id}").
			// If the pattern is empty (404 Not Found), we use the path or the label "unknown".
			pathPattern := r.Pattern
			if pathPattern == "" {
				pathPattern = r.URL.Path
			} else {
				// Removing the method from r.Pattern (например, "GET /orders/{id}" -> "/orders/{id}")
				parts := strings.SplitN(pathPattern, " ", 2)
				if len(parts) == 2 {
					pathPattern = parts[1]
				}
			}

			statusCodeStr := strconv.Itoa(wrapped.statusCode)

			// Increment the request counter
			m.HTTPRequestsTotal.WithLabelValues(r.Method, pathPattern, statusCodeStr).Inc()

			// We record the request duration in a histogram
			m.HTTPRequestDuration.WithLabelValues(r.Method, pathPattern).Observe(duration)
		})
	}
}
