package domain

import (
	"errors"

	"github.com/google/uuid"
)

var (
	ErrInvalidPrice     = errors.New("price must be greater than 0")
	ErrEmptyProductName = errors.New("product name cannot be empty")
	ErrProductNotFound  = errors.New("product not found")
)

type Product struct {
	ID    uuid.UUID
	Name  string
	Price float64
}

func NewProduct(name string, price float64) (Product, error) {
	if name == "" {
		return Product{}, ErrEmptyName
	}
	if price <= 0 {
		return Product{}, ErrInvalidPrice
	}

	return Product{
		ID:    uuid.New(),
		Name:  name,
		Price: price,
	}, nil
}

func (p *Product) ChangePrice(price float64) error {
	if price <= 0 {
		return ErrInvalidPrice
	}
	p.Price = price
	return nil
}
