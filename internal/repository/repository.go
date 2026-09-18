package repository

import (
	"errors"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("entity not found")
)

type UserRepository interface {
	Save(user domain.User) error
	GetByID(id uuid.UUID) (domain.User, error)
}

type ProductRepository interface {
	Save(product domain.Product) error
	GetByID(id uuid.UUID) (domain.Product, error)
	List(limit, offset int) ([]domain.Product, error)
}

type OrderRepository interface {
	Save(order domain.Order) error
	GetByID(id uuid.UUID) (domain.Order, error)
}

type PaymentRepository interface {
	Save(payment domain.Payment) error
	GetByID(id uuid.UUID) (domain.Payment, error)
}
