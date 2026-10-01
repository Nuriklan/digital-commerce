package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/pkg/auth"
)

var (
	ErrUserAlreadyExists = errors.New("user with this email already exists")
)

type AuthService struct {
	userRepo repository.UserRepository
	jwtMgr   *auth.JWTManager
}

func NewAuthService(userRepo repository.UserRepository, jwtMgr *auth.JWTManager) *AuthService {
	return &AuthService{
		userRepo: userRepo,
		jwtMgr:   jwtMgr,
	}
}

func (s *AuthService) Register(ctx context.Context, name, email, password string, role domain.Role) (domain.User, string, error) {
	// Check if user already exists
	_, err := s.userRepo.GetByEmail(ctx, email)
	if err == nil {
		return domain.User{}, "", ErrUserAlreadyExists
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return domain.User{}, "", fmt.Errorf("failed to check existing user: %w", err)
	}

	user, err := domain.NewUserWithPassword(name, email, password, role)
	if err != nil {
		return domain.User{}, "", err
	}

	if err := s.userRepo.Save(ctx, user); err != nil {
		return domain.User{}, "", fmt.Errorf("failed to save user: %w", err)
	}

	token, err := s.jwtMgr.Generate(user)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("failed to generate token: %w", err)
	}

	return user, token, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (domain.User, string, error) {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return domain.User{}, "", domain.ErrInvalidCredentials
		}
		return domain.User{}, "", fmt.Errorf("failed to get user: %w", err)
	}

	if !user.CheckPassword(password) {
		return domain.User{}, "", domain.ErrInvalidCredentials
	}

	token, err := s.jwtMgr.Generate(user)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("failed to generate token: %w", err)
	}

	return user, token, nil
}
