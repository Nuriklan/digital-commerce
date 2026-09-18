package service

import (
	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/google/uuid"
)

type UserService struct {
	repo repository.UserRepository
}

func NewUserService(repo repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) CreateUser(name, email string) (domain.User, error) {
	user, err := domain.NewUser(name, email)
	if err != nil {
		return domain.User{}, err
	}

	if err := s.repo.Save(user); err != nil {
		return domain.User{}, err
	}

	return user, nil
}

func (s *UserService) GetUser(id uuid.UUID) (domain.User, error) {
	return s.repo.GetByID(id)
}
