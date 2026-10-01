package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/Nuriklan/digital-commerce/pkg/auth"
)

func TestAuthService_RegisterAndLogin_Success(t *testing.T) {
	ctx := context.Background()
	userRepo := repository.NewUserMemoryRepository()
	jwtMgr := auth.NewJWTManager("test-secret-key-12345678", 15*time.Minute)
	authSvc := service.NewAuthService(userRepo, jwtMgr)

	// 1. Registration
	user, token, err := authSvc.Register(ctx, "Bob", "bob@example.com", "password123", domain.RoleUser)
	if err != nil {
		t.Fatalf("unexpected registration error: %v", err)
	}
	if user.Email != "bob@example.com" || token == "" {
		t.Fatalf("invalid registered user data")
	}

	// 2. Enter with wrong credentials
	loginUser, loginToken, err := authSvc.Login(ctx, "bob@example.com", "password123")
	if err != nil {
		t.Fatalf("unexpected login error: %v", err)
	}
	if loginUser.ID != user.ID || loginToken == "" {
		t.Errorf("login failed to return valid user/token")
	}

	// 3. Enter incorrect password
	_, _, err = authSvc.Login(ctx, "bob@example.com", "wrongpass")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials for wrong password, got: %v", err)
	}

	// 4. Re-registration with the same email
	_, _, err = authSvc.Register(ctx, "Bob 2", "bob@example.com", "password123", domain.RoleUser)
	if !errors.Is(err, service.ErrUserAlreadyExists) {
		t.Errorf("expected ErrUserAlreadyExists, got: %v", err)
	}
}
