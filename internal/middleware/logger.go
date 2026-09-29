package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Nuriklan/digital-commerce/pkg/tracing"
)

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func newResponseWriterWrapper(w http.ResponseWriter) *responseWriterWrapper {
	return &responseWriterWrapper{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

func Logger(next http.Handler) http.Handler {
	return LoggerWith(slog.Default())(next)
}

func LoggerWith(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := newResponseWriterWrapper(w)

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)
			reqID := GetRequestID(r.Context())
			traceID := tracing.TraceIDFromContext(r.Context())

			level := slog.LevelInfo
			if wrapped.statusCode >= http.StatusInternalServerError {
				level = slog.LevelError
			} else if wrapped.statusCode >= http.StatusBadRequest {
				level = slog.LevelWarn
			}

			attrs := []slog.Attr{
				slog.String("request_id", reqID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", wrapped.statusCode),
				slog.Duration("duration", duration),
				slog.Int64("bytes", wrapped.bytesWritten),
				slog.String("remote_addr", r.RemoteAddr),
			}

			if traceID != "" {
				attrs = append(attrs, slog.String("trace_id", traceID))
			}

			logger.LogAttrs(r.Context(), level, "http request completed", attrs...)
		})
	}
}
