package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/middleware"
)

func TestLoggerMiddleware_StructuredOutput(t *testing.T) {
	tests := []struct {
		name          string
		handlerStatus int
		expectedLevel string
	}{
		{
			name:          "200 OK -> INFO level",
			handlerStatus: http.StatusOK,
			expectedLevel: "INFO",
		},
		{
			name:          "404 Not Found -> WARN level",
			handlerStatus: http.StatusNotFound,
			expectedLevel: "WARN",
		},
		{
			name:          "500 Internal Server Error -> ERROR level",
			handlerStatus: http.StatusInternalServerError,
			expectedLevel: "ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			testLogger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))

			dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.handlerStatus)
				w.Write([]byte("response"))
			})

			chain := middleware.Chain(
				dummyHandler,
				middleware.RequestID,
				middleware.LoggerWith(testLogger),
			)

			req := httptest.NewRequest(http.MethodGet, "/orders/123", nil)
			req.Header.Set(middleware.HeaderXRequestID, "test-req-xyz")
			rec := httptest.NewRecorder()

			chain.ServeHTTP(rec, req)

			var entry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				t.Fatalf("failed to decode log json: %v, raw: %s", err, buf.String())
			}

			if entry["level"] != tt.expectedLevel {
				t.Errorf("expected level %q, got %q", tt.expectedLevel, entry["level"])
			}

			if entry["request_id"] != "test-req-xyz" {
				t.Errorf("expected request_id 'test-req-xyz', got: %v", entry["request_id"])
			}

			if entry["method"] != "GET" {
				t.Errorf("expected method GET, got: %v", entry["method"])
			}
			if entry["path"] != "/orders/123" {
				t.Errorf("expected path /orders/123, got: %v", entry["path"])
			}
			if int(entry["status"].(float64)) != tt.handlerStatus {
				t.Errorf("expected status %d, got: %v", tt.handlerStatus, entry["status"])
			}
		})
	}
}
