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
	if _, err := s.userRepo.GetByID(ctx, userID); err != nil {
		return domain.Order{}, fmt.Errorf("user not found: %w", err)
	}

	order := domain.NewOrder(userID)

	for _, item := range items {
		if item.Quantity <= 0 {
			return domain.Order{}, ErrInvalidQuantity
		}

		product, err := s.productRepo.GetByID(ctx, item.ProductID)
		if err != nil {
			return domain.Order{}, fmt.Errorf("product %s not found: %w", item.ProductID, err)
		}

		order.AddItem(product, item.Quantity)
	}

	if err := s.orderRepo.Save(ctx, order); err != nil {
		return domain.Order{}, fmt.Errorf("failed to save order: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.PublishOrderCreated(ctx, order)
	}

	return order, nil
}

func (s *OrderService) GetOrder(ctx context.Context, id uuid.UUID) (domain.Order, error) {
	return s.orderRepo.GetByID(ctx, id)
}

func (s *OrderService) CancelOrder(ctx context.Context, id uuid.UUID) (domain.Order, error) {
	order, err := s.orderRepo.GetByIDForUpdate(ctx, id)
	if err != nil {
		return domain.Order{}, err
	}

	if err := order.Cancel(); err != nil {
		return domain.Order{}, err
	}

	if err := s.orderRepo.Save(ctx, order); err != nil {
		return domain.Order{}, err
	}

	return order, nil
}
