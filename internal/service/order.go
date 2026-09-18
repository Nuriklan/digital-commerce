package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrInvalidQuantity = errors.New("quantity must be greater than 0")
)

type CreateOrderItemDTO struct {
	ProductID uuid.UUID
	Quantity  int
}

type OrderService struct {
	orderRepo   repository.OrderRepository
	userRepo    repository.UserRepository
	productRepo repository.ProductRepository
	publisher   OrderEventPublisher
}

func NewOrderService(
	orderRepo repository.OrderRepository,
	userRepo repository.UserRepository,
	productRepo repository.ProductRepository,
	publisher OrderEventPublisher,
) *OrderService {
	return &OrderService{
		orderRepo:   orderRepo,
		userRepo:    userRepo,
		productRepo: productRepo,
		publisher:   publisher,
	}
}

func (s *OrderService) CreateOrder(ctx context.Context, userID uuid.UUID, items []CreateOrderItemDTO) (domain.Order, error) {
	if _, err := s.userRepo.GetByID(userID); err != nil {
		return domain.Order{}, fmt.Errorf("user not found: %w", err)
	}

	order := domain.NewOrder(userID)

	for _, item := range items {
		if item.Quantity <= 0 {
			return domain.Order{}, ErrInvalidQuantity
		}

		product, err := s.productRepo.GetByID(item.ProductID)
		if err != nil {
			return domain.Order{}, fmt.Errorf("product %s not found: %w", item.ProductID, err)
		}

		order.AddItem(product, item.Quantity)
	}

	if err := s.orderRepo.Save(order); err != nil {
		return domain.Order{}, fmt.Errorf("failed to save order: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.PublishOrderCreated(ctx, order)
	}

	return order, nil
}

func (s *OrderService) GetOrder(id uuid.UUID) (domain.Order, error) {
	return s.orderRepo.GetByID(id)
}

func (s *OrderService) CancelOrder(id uuid.UUID) (domain.Order, error) {
	order, err := s.orderRepo.GetByID(id)
	if err != nil {
		return domain.Order{}, err
	}

	if err := order.Cancel(); err != nil {
		return domain.Order{}, err
	}

	if err := s.orderRepo.Save(order); err != nil {
		return domain.Order{}, err
	}

	return order, nil
}
