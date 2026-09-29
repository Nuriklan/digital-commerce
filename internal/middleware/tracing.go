package middleware

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

const HeaderXTraceID = "X-Trace-ID"

func Tracing(serviceName string) func(http.Handler) http.Handler {
	tracer := otel.GetTracerProvider().Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			spanName := fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path)
			ctx, span := tracer.Start(
				ctx,
				spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPMethodKey.String(r.Method),
					semconv.HTTPTargetKey.String(r.URL.Path),
					semconv.HTTPRouteKey.String(r.Pattern),
					semconv.UserAgentOriginalKey.String(r.UserAgent()),
				),
			)
			defer span.End()

			if span.SpanContext().IsValid() {
				w.Header().Set(HeaderXTraceID, span.SpanContext().TraceID().String())
			}

			wrapped := newResponseWriterWrapper(w)

			next.ServeHTTP(wrapped, r.WithContext(ctx))

			span.SetAttributes(semconv.HTTPStatusCodeKey.Int(wrapped.statusCode))
			if wrapped.statusCode >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP error: %d", wrapped.statusCode))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}
