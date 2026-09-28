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

// mockProductCatalog — a mock of the service.ProductCatalog interface using the function-field pattern.
type mockProductCatalog struct {
	getByIDFunc func(ctx context.Context, id uuid.UUID) (domain.Product, error)
}

func (m *mockProductCatalog) GetByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return domain.Product{}, domain.ErrProductNotFound
}

// mockTxManager — transaction manager mock.
// Allows simulating both normal transaction execution and commit/execution failure.
type mockTxManager struct {
	withinTxFunc func(ctx context.Context, fn func(txCtx context.Context) error) error
}

func (m *mockTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	if m.withinTxFunc != nil {
		return m.withinTxFunc(ctx, fn)
	}
	return fn(ctx)
}

// TestOrderService_CreateOrder_TableDriven tests order creation scenarios with dependency isolation.
func TestOrderService_CreateOrder_TableDriven(t *testing.T) {
	testUserID := uuid.New()
	validProduct := domain.Product{
		ID:    uuid.New(),
		Name:  "Clean Code in Go",
		Price: 45.0,
	}

	errCatalogUnavailable := errors.New("catalog gRPC service unreachable")
	errTxFailed := errors.New("database transaction deadlock")

	tests := []struct {
		name          string
		userID        uuid.UUID
		items         []service.CreateOrderItemDTO
		setupUserRepo func(repo *repository.UserMemoryRepository)
		catalogMock   *mockProductCatalog
		txMock        *mockTxManager
		expectedErr   error
		verify        func(t *testing.T, order domain.Order, outbox *repository.OutboxMemoryRepository)
	}{
		{
			name:   "success: creates order and outbox record in transaction",
			userID: testUserID,
			items: []service.CreateOrderItemDTO{
				{ProductID: validProduct.ID, Quantity: 2},
			},
			setupUserRepo: func(repo *repository.UserMemoryRepository) {
				user, _ := domain.NewUser("Alice", "alice@example.com")
				user.ID = testUserID
				_ = repo.Save(context.Background(), user)
			},
			catalogMock: &mockProductCatalog{
				getByIDFunc: func(ctx context.Context, id uuid.UUID) (domain.Product, error) {
					if id == validProduct.ID {
						return validProduct, nil
					}
					return domain.Product{}, domain.ErrProductNotFound
				},
			},
			txMock: &mockTxManager{
				withinTxFunc: func(ctx context.Context, fn func(txCtx context.Context) error) error {
					return fn(ctx)
				},
			},
			expectedErr: nil,
			verify: func(t *testing.T, order domain.Order, outbox *repository.OutboxMemoryRepository) {
				if order.UserID != testUserID {
					t.Errorf("expected user ID %s, got %s", testUserID, order.UserID)
				}
				expectedTotal := 90.0
				if total := order.CalculateTotal(); total != expectedTotal {
					t.Errorf("expected total %.2f, got %.2f", expectedTotal, total)
				}
				records := outbox.GetAll()
				if len(records) != 1 {
					t.Fatalf("expected 1 outbox record, got %d", len(records))
				}
				if records[0].AggregateID != order.ID {
					t.Errorf("expected outbox aggregate %s, got %s", order.ID, records[0].AggregateID)
				}
			},
		},
		{
			name:   "failure: user does not exist",
			userID: uuid.New(), // unknown ID
			items: []service.CreateOrderItemDTO{
				{ProductID: validProduct.ID, Quantity: 1},
			},
			setupUserRepo: func(repo *repository.UserMemoryRepository) {
				// Leaving user repository empty
			},
			catalogMock: &mockProductCatalog{},
			expectedErr: repository.ErrNotFound,
		},
		{
			name:   "failure: invalid quantity in item",
			userID: testUserID,
			items: []service.CreateOrderItemDTO{
				{ProductID: validProduct.ID, Quantity: 0},
			},
			setupUserRepo: func(repo *repository.UserMemoryRepository) {
				user, _ := domain.NewUser("Alice", "alice@example.com")
				user.ID = testUserID
				_ = repo.Save(context.Background(), user)
			},
			catalogMock: &mockProductCatalog{},
			expectedErr: service.ErrInvalidQuantity,
		},
		{
			name:   "failure: catalog service unavailable",
			userID: testUserID,
			items: []service.CreateOrderItemDTO{
				{ProductID: validProduct.ID, Quantity: 1},
			},
			setupUserRepo: func(repo *repository.UserMemoryRepository) {
				user, _ := domain.NewUser("Alice", "alice@example.com")
				user.ID = testUserID
				_ = repo.Save(context.Background(), user)
			},
			catalogMock: &mockProductCatalog{
				getByIDFunc: func(ctx context.Context, id uuid.UUID) (domain.Product, error) {
					return domain.Product{}, errCatalogUnavailable
				},
			},
			expectedErr: errCatalogUnavailable,
		},
		{
			name:   "failure: database transaction fails",
			userID: testUserID,
			items: []service.CreateOrderItemDTO{
				{ProductID: validProduct.ID, Quantity: 1},
			},
			setupUserRepo: func(repo *repository.UserMemoryRepository) {
				user, _ := domain.NewUser("Alice", "alice@example.com")
				user.ID = testUserID
				_ = repo.Save(context.Background(), user)
			},
			catalogMock: &mockProductCatalog{
				getByIDFunc: func(ctx context.Context, id uuid.UUID) (domain.Product, error) {
					return validProduct, nil
				},
			},
			txMock: &mockTxManager{
				withinTxFunc: func(ctx context.Context, fn func(txCtx context.Context) error) error {
					return errTxFailed
				},
			},
			expectedErr: errTxFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRepo := repository.NewUserMemoryRepository()
			if tt.setupUserRepo != nil {
				tt.setupUserRepo(userRepo)
			}

			orderRepo := repository.NewOrderMemoryRepository()
			outboxRepo := repository.NewOutboxMemoryRepository()

			svc := service.NewOrderService(
				orderRepo,
				userRepo,
				tt.catalogMock,
				tt.txMock,
				outboxRepo,
			)

			order, err := svc.CreateOrder(context.Background(), tt.userID, tt.items)

			if tt.expectedErr != nil {
				if !errors.Is(err, tt.expectedErr) {
					t.Errorf("expected error %v, got %v", tt.expectedErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.verify != nil {
				tt.verify(t, order, outboxRepo)
			}
		})
	}
}

// TestOrderService_CancelOrder verifies order cancellation scenarios.
func TestOrderService_CancelOrder(t *testing.T) {
	orderRepo := repository.NewOrderMemoryRepository()
	svc := service.NewOrderService(orderRepo, nil, nil, nil, nil)

	// 1. Successful cancellation of a pending order
	order := domain.NewOrder(uuid.New())
	_ = orderRepo.Save(context.Background(), order)

	cancelledOrder, err := svc.CancelOrder(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("unexpected error cancelling order: %v", err)
	}
	if cancelledOrder.Status != domain.OrderStatusCancelled {
		t.Errorf("expected status cancelled, got %s", cancelledOrder.Status)
	}

	// 2. Canceling an already cancelled order again must return ErrOrderAlreadyCancelled
	_, err = svc.CancelOrder(context.Background(), order.ID)
	if !errors.Is(err, domain.ErrOrderAlreadyCancelled) {
		t.Errorf("expected ErrOrderAlreadyCancelled, got %v", err)
	}

	// 3. Canceling a non-existent order should return ErrNotFound
	_, err = svc.CancelOrder(context.Background(), uuid.New())
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
