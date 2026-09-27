package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/pkg/events"
	"github.com/google/uuid"
)

var (
	ErrInvalidQuantity = errors.New("quantity must be greater than 0")
)

type CreateOrderItemDTO struct {
	ProductID uuid.UUID
	Quantity  int
}

type ProductCatalog interface {
	GetByID(ctx context.Context, id uuid.UUID) (domain.Product, error)
}

type OrderService struct {
	orderRepo  repository.OrderRepository
	userRepo   repository.UserRepository
	catalog    ProductCatalog
	txManager  repository.TxManager
	outboxRepo repository.OutboxRepository
}

func NewOrderService(
	orderRepo repository.OrderRepository,
	userRepo repository.UserRepository,
	catalog ProductCatalog,
	txManager repository.TxManager,
	outboxRepo repository.OutboxRepository,
) *OrderService {
	return &OrderService{
		orderRepo:  orderRepo,
		userRepo:   userRepo,
		catalog:    catalog,
		txManager:  txManager,
		outboxRepo: outboxRepo,
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

		product, err := s.catalog.GetByID(ctx, item.ProductID)
		if err != nil {
			return domain.Order{}, fmt.Errorf("product %s not found: %w", item.ProductID, err)
		}

		order.AddItem(product, item.Quantity)
	}

	// 1. Creating an OrderCreatedEvent for publication
	eventItems := make([]events.OrderItemPayload, len(order.Items))
	for i, it := range order.Items {
		eventItems[i] = events.OrderItemPayload{
			ProductID:   it.ProductID,
			ProductName: it.Name,
			Price:       it.Price,
			Quantity:    it.Quantity,
		}
	}

	event := events.NewOrderCreatedEvent(order.ID, order.UserID, order.CalculateTotal(), eventItems)
	payload, err := event.Marshal()
	if err != nil {
		return domain.Order{}, fmt.Errorf("failed to marshal order created event: %w", err)
	}

	// 2. Creating an Outbox record
	outboxRecord := domain.NewOutboxRecord("order", order.ID, events.TopicOrderEvents, payload)

	// 3. Atomically saving the Order and OutboxRecord within a single database transaction
	saveFn := func(txCtx context.Context) error {
		if err := s.orderRepo.Save(txCtx, order); err != nil {
			return fmt.Errorf("failed to save order: %w", err)
		}

		if s.outboxRepo != nil {
			if err := s.outboxRepo.Save(txCtx, outboxRecord); err != nil {
				return fmt.Errorf("failed to save outbox record: %w", err)
			}
		}

		return nil
	}

	if s.txManager != nil {
		if err := s.txManager.WithinTransaction(ctx, saveFn); err != nil {
			return domain.Order{}, err
		}
	} else {
		if err := saveFn(ctx); err != nil {
			return domain.Order{}, err
		}
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
