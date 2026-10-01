package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/pkg/auth"
	"github.com/google/uuid"
)

type userContextKey struct{}

func WithUserClaims(ctx context.Context, claims *auth.UserClaims) context.Context {
	return context.WithValue(ctx, userContextKey{}, claims)
}

func GetUserClaims(ctx context.Context) (*auth.UserClaims, bool) {
	claims, ok := ctx.Value(userContextKey{}).(*auth.UserClaims)
	return claims, ok
}

func GetUserID(ctx context.Context) (uuid.UUID, bool) {
	claims, ok := GetUserClaims(ctx)
	if !ok || claims == nil {
		return uuid.Nil, false
	}
	return claims.UserID, true
}

func JWTAuth(jwtMgr *auth.JWTManager) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				respondJSONError(w, http.StatusUnauthorized, "missing Authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				respondJSONError(w, http.StatusUnauthorized, "invalid authorization header format, expected 'Bearer <token>'")
				return
			}

			claims, err := jwtMgr.Verify(parts[1])
			if err != nil {
				respondJSONError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := WithUserClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireRole(allowedRoles ...domain.Role) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := GetUserClaims(r.Context())
			if !ok || claims == nil {
				respondJSONError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			hasRole := false
			for _, allowed := range allowedRoles {
				if claims.Role == allowed {
					hasRole = true
					break
				}
			}

			if !hasRole {
				respondJSONError(w, http.StatusForbidden, "forbidden: insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func respondJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": msg,
	})
}
