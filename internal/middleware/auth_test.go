package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/middleware"
	"github.com/Nuriklan/digital-commerce/pkg/auth"
)

func TestJWTAuthMiddleware(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret-key-12345678", 15*time.Minute)
	user, _ := domain.NewUserWithPassword("Alice", "alice@example.com", "pass123", domain.RoleUser)
	token, _ := jwtMgr.Generate(user)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.GetUserClaims(r.Context())
		if !ok || claims.UserID != user.ID {
			t.Errorf("failed to retrieve user claims in downstream handler")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.JWTAuth(jwtMgr)(nextHandler)

	t.Run("valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("missing authorization header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})
}

func TestRequireRoleMiddleware(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret-key-12345678", 15*time.Minute)
	adminUser, _ := domain.NewUserWithPassword("Admin", "admin@example.com", "pass123", domain.RoleAdmin)
	adminToken, _ := jwtMgr.Generate(adminUser)

	regularUser, _ := domain.NewUserWithPassword("User", "user@example.com", "pass123", domain.RoleUser)
	userToken, _ := jwtMgr.Generate(regularUser)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	adminOnlyHandler := middleware.Chain(
		nextHandler,
		middleware.JWTAuth(jwtMgr),
		middleware.RequireRole(domain.RoleAdmin),
	)

	t.Run("admin access allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()

		adminOnlyHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK for admin, got %d", rec.Code)
		}
	})

	t.Run("regular user access forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)
		rec := httptest.NewRecorder()

		adminOnlyHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for regular user, got %d", rec.Code)
		}
	})
}

func TestCORSMiddleware(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	corsHandler := middleware.CORS(nextHandler)

	t.Run("preflight options request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/orders", nil)
		rec := httptest.NewRecorder()

		corsHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("expected 204 No Content for OPTIONS, got %d", rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("expected Access-Control-Allow-Origin header")
		}
	})
}
