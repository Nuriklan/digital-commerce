package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/middleware"
	"github.com/Nuriklan/digital-commerce/pkg/tracing"
)

func TestTracingMiddleware(t *testing.T) {
	tp, err := tracing.InitTracer("test-service", nil)
	if err != nil {
		t.Fatalf("failed to init tracer: %v", err)
	}
	defer tp.Shutdown(context.Background())

	var capturedTraceID string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceID = tracing.TraceIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	tracingMW := middleware.Tracing("test-service")
	h := tracingMW(handler)

	req := httptest.NewRequest(http.MethodGet, "/catalog/products", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if capturedTraceID == "" {
		t.Errorf("expected valid trace ID in context, got empty string")
	}

	respTraceID := rec.Header().Get(middleware.HeaderXTraceID)
	if respTraceID != capturedTraceID {
		t.Errorf("expected header %s to match %s, got: %s", middleware.HeaderXTraceID, capturedTraceID, respTraceID)
	}
}

func TestTracingMiddleware_PropagatesW3C(t *testing.T) {
	tp, err := tracing.InitTracer("test-service", nil)
	if err != nil {
		t.Fatalf("failed to init tracer: %v", err)
	}
	defer tp.Shutdown(context.Background())

	var capturedTraceID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceID = tracing.TraceIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	h := middleware.Tracing("test-service")(handler)

	req := httptest.NewRequest(http.MethodGet, "/catalog/products", nil)
	incomingTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	req.Header.Set("traceparent", "00-"+incomingTraceID+"-00f067aa0ba902b7-01")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if capturedTraceID != incomingTraceID {
		t.Errorf("expected extracted traceID to be %s, got %s", incomingTraceID, capturedTraceID)
	}
}
