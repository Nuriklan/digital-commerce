package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Role string

const (
	RoleUser  Role = "USER"
	RoleAdmin Role = "ADMIN"
)

func (r Role) IsValid() bool {
	switch r {
	case RoleUser, RoleAdmin:
		return true
	default:
		return false
	}
}

var (
	ErrEmptyEmail         = errors.New("email cannot be empty")
	ErrEmptyName          = errors.New("name cannot be empty")
	ErrPasswordTooShort   = errors.New("password must be at least 6 characters")
	ErrInvalidRole        = errors.New("invalid user role")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

func NewUser(name, email string) (User, error) {
	return NewUserWithPassword(name, email, "default_password", RoleUser)
}

func NewUserWithPassword(name, email, plainPassword string, role Role) (User, error) {
	if name == "" {
		return User{}, ErrEmptyName
	}
	if email == "" {
		return User{}, ErrEmptyEmail
	}
	if len(plainPassword) < 6 {
		return User{}, ErrPasswordTooShort
	}
	if !role.IsValid() {
		return User{}, ErrInvalidRole
	}

	hash, err := HashPassword(plainPassword)
	if err != nil {
		return User{}, err
	}

	return User{
		ID:           uuid.New(),
		Name:         name,
		Email:        email,
		PasswordHash: hash,
		Role:         role,
		CreatedAt:    time.Now(),
	}, nil
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (u User) CheckPassword(plainPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(plainPassword))
	return err == nil
}
