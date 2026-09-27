package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/google/uuid"
)

type mockPublisher struct {
	publishedOrders []domain.Order
}

func (m *mockPublisher) PublishOrderCreated(ctx context.Context, order domain.Order) error {
	m.publishedOrders = append(m.publishedOrders, order)
	return nil
}

func TestOrderService_CreateOrder_Success(t *testing.T) {
	// Arrange
	userRepo := repository.NewUserMemoryRepository()
	productRepo := repository.NewProductMemoryRepository()
	orderRepo := repository.NewOrderMemoryRepository()
	txManager := repository.NewMemoryTxManager()
	outboxRepo := repository.NewOutboxMemoryRepository()

	svc := service.NewOrderService(orderRepo, userRepo, productRepo, txManager, outboxRepo)

	user, _ := domain.NewUser("Alice", "alice@example.com")
	product, _ := domain.NewProduct("Go in Action Book", 35.0)
	_ = userRepo.Save(context.Background(), user)
	_ = productRepo.Save(context.Background(), product)

	items := []service.CreateOrderItemDTO{
		{ProductID: product.ID, Quantity: 2},
	}

	// Act
	order, err := svc.CreateOrder(context.Background(), user.ID, items)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if order.UserID != user.ID {
		t.Errorf("expected user ID %s, got %s", user.ID, order.UserID)
	}

	expectedTotal := 70.0
	if order.CalculateTotal() != expectedTotal {
		t.Errorf("expected total %.2f, got %.2f", expectedTotal, order.CalculateTotal())
	}

	savedOrder, err := orderRepo.GetByID(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("expected order to be in repository: %v", err)
	}
	if savedOrder.ID != order.ID {
		t.Errorf("expected saved order ID %s, got %s", order.ID, savedOrder.ID)
	}

	outboxRecords := outboxRepo.GetAll()
	if len(outboxRecords) != 1 {
		t.Fatalf("expected 1 outbox record, got %d", len(outboxRecords))
	}
	if outboxRecords[0].AggregateID != order.ID {
		t.Errorf("expected outbox aggregate ID %s, got %s", order.ID, outboxRecords[0].AggregateID)
	}
}

func TestOrderService_CreateOrder_UserNotFound(t *testing.T) {
	userRepo := repository.NewUserMemoryRepository()
	productRepo := repository.NewProductMemoryRepository()
	orderRepo := repository.NewOrderMemoryRepository()

	svc := service.NewOrderService(orderRepo, userRepo, productRepo, nil, nil)

	randomUserID := uuid.New()
	_, err := svc.CreateOrder(context.Background(), randomUserID, nil)

	if err == nil {
		t.Fatal("expected error for non-existent user, got nil")
	}

	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected errors.Is(err, repository.ErrNotFound) to be true, got %v", err)
	}
}

func TestOrderService_CreateOrder_InvalidQuantity(t *testing.T) {
	userRepo := repository.NewUserMemoryRepository()
	productRepo := repository.NewProductMemoryRepository()
	orderRepo := repository.NewOrderMemoryRepository()

	svc := service.NewOrderService(orderRepo, userRepo, productRepo, nil, nil)

	user, _ := domain.NewUser("Alice", "alice@example.com")
	_ = userRepo.Save(context.Background(), user)

	items := []service.CreateOrderItemDTO{
		{ProductID: uuid.New(), Quantity: -1},
	}

	_, err := svc.CreateOrder(context.Background(), user.ID, items)

	if !errors.Is(err, service.ErrInvalidQuantity) {
		t.Errorf("expected ErrInvalidQuantity, got %v", err)
	}
}
