package service

import (
	"context"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/google/uuid"
)

type ProductService struct {
	repo repository.ProductRepository
}

func NewProductService(repo repository.ProductRepository) *ProductService {
	return &ProductService{repo: repo}
}

func (s *ProductService) CreateProduct(ctx context.Context, name string, price float64) (domain.Product, error) {
	product, err := domain.NewProduct(name, price)
	if err != nil {
		return domain.Product{}, err
	}

	if err := s.repo.Save(ctx, product); err != nil {
		return domain.Product{}, err
	}

	return product, nil
}

func (s *ProductService) GetProduct(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ProductService) ListProducts(ctx context.Context, limit, offset int) ([]domain.Product, error) {
	return s.repo.List(ctx, limit, offset)
}
