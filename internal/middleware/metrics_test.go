package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/middleware"
	"github.com/Nuriklan/digital-commerce/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsMiddleware_CollectsMetrics(t *testing.T) {
	m := metrics.New("gateway-test")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("order details"))
	})

	// Wrap the router in the Metrics middleware
	h := middleware.Metrics(m)(mux)

	// We are making a test request
	req := httptest.NewRequest(http.MethodGet, "/orders/42", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got: %d", rec.Code)
	}

	// Checking the counter value using prometheus/testutil
	count := testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("GET", "/orders/{id}", "200"))
	if count != 1.0 {
		t.Errorf("expected 1 request recorded, got %v", count)
	}

	// Verify that the /metrics handler returns data
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	m.Handler().ServeHTTP(metricsRec, metricsReq)

	if metricsRec.Code != http.StatusOK {
		t.Errorf("expected /metrics status 200, got: %d", metricsRec.Code)
	}

	body := metricsRec.Body.String()
	if !strings.Contains(body, "digital_commerce_http_requests_total") {
		t.Errorf("expected metric 'digital_commerce_http_requests_total' in /metrics output, got:\n%s", body)
	}
}
