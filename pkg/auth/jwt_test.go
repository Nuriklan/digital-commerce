package auth_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/pkg/auth"
)

func TestJWTManager_GenerateAndVerify_Success(t *testing.T) {
	secretKey := "super-secret-secure-key-12345"
	jwtMgr := auth.NewJWTManager(secretKey, 15*time.Minute)

	user, err := domain.NewUserWithPassword("Admin", "admin@example.com", "secure123", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	tokenStr, err := jwtMgr.Generate(user)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}
	if tokenStr == "" {
		t.Fatalf("expected non-empty token")
	}

	claims, err := jwtMgr.Verify(tokenStr)
	if err != nil {
		t.Fatalf("unexpected error verifying token: %v", err)
	}

	if claims.UserID != user.ID {
		t.Errorf("expected userID %s, got %s", user.ID, claims.UserID)
	}
	if claims.Email != user.Email {
		t.Errorf("expected email %s, got %s", user.Email, claims.Email)
	}
	if claims.Role != domain.RoleAdmin {
		t.Errorf("expected role %s, got %s", domain.RoleAdmin, claims.Role)
	}
}

func TestJWTManager_Verify_ExpiredToken(t *testing.T) {
	secretKey := "super-secret-secure-key-12345"
	jwtMgr := auth.NewJWTManager(secretKey, -1*time.Minute)

	user, err := domain.NewUserWithPassword("Bob", "bob@example.com", "secure123", domain.RoleUser)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	tokenStr, err := jwtMgr.Generate(user)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = jwtMgr.Verify(tokenStr)
	if !errors.Is(err, auth.ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestJWTManager_Verify_InvalidSignature(t *testing.T) {
	manager1 := auth.NewJWTManager("secret-key-1", 15*time.Minute)
	manager2 := auth.NewJWTManager("secret-key-2", 15*time.Minute)

	user, _ := domain.NewUserWithPassword("Charlie", "charlie@example.com", "secure123", domain.RoleUser)
	tokenStr, _ := manager1.Generate(user)

	_, err := manager2.Verify(tokenStr)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for invalid signature, got %v", err)
	}
}

func TestJWTManager_Verify_MalformedToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager("some-secret", 15*time.Minute)

	_, err := jwtMgr.Verify("not-a-valid-jwt-token")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for malformed string, got %v", err)
	}
}
