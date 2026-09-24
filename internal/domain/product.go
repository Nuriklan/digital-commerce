package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidPrice     = errors.New("price must be greater than 0")
	ErrEmptyProductName = errors.New("product name cannot be empty")
	ErrProductNotFound  = errors.New("product not found")
)

type Product struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Price     float64   `json:"price"`
	CreatedAt time.Time `json:"created_at"`
}

func NewProduct(name string, price float64) (Product, error) {
	if name == "" {
		return Product{}, ErrEmptyName
	}
	if price <= 0 {
		return Product{}, ErrInvalidPrice
	}

	return Product{
		ID:        uuid.New(),
		Name:      name,
		Price:     price,
		CreatedAt: time.Now(),
	}, nil
}

func (p *Product) ChangePrice(price float64) error {
	if price <= 0 {
		return ErrInvalidPrice
	}
	p.Price = price
	return nil
}
