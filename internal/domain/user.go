package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrEmptyEmail = errors.New("email cannot be empty")
	ErrEmptyName  = errors.New("name cannot be empty")
)

type User struct {
	ID        uuid.UUID
	Name      string
	Email     string
	CreatedAt time.Time
}

func NewUser(name, email string) (User, error) {
	if name == "" {
		return User{}, ErrEmptyName
	}
	if email == "" {
		return User{}, ErrEmptyEmail
	}

	return User{
		ID:        uuid.New(),
		Name:      name,
		Email:     email,
		CreatedAt: time.Now(),
	}, nil
}
