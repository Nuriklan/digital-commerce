package service

import (
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

func (s *ProductService) CreateProduct(name string, price float64) (domain.Product, error) {
	product, err := domain.NewProduct(name, price)
	if err != nil {
		return domain.Product{}, err
	}

	if err := s.repo.Save(product); err != nil {
		return domain.Product{}, err
	}

	return product, nil
}

func (s *ProductService) GetProduct(id uuid.UUID) (domain.Product, error) {
	return s.repo.GetByID(id)
}

func (s *ProductService) ListProducts(limit, offset int) ([]domain.Product, error) {
	return s.repo.List(limit, offset)
}
