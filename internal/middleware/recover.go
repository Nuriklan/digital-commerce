package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
)

func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				reqID := GetRequestID(r.Context())
				log.Printf("[%s] PANIC RECOVERED: %v\nStacktrace:\n%s", reqID,
					rec, string(debug.Stack()))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":      "Internal Server Error",
					"request_id": reqID,
					"details":    fmt.Sprintf("%v", rec),
				})
			}
		}()

		next.ServeHTTP(w, r)
	})
}
