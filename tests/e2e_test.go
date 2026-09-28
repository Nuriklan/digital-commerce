package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/handler"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/saga"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/Nuriklan/digital-commerce/internal/worker"
	"github.com/google/uuid"
)

type inMemoryEventBroker struct {
	mu       sync.Mutex
	messages [][]byte
}

func (b *inMemoryEventBroker) Publish(ctx context.Context, key string, payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages = append(b.messages, payload)
	return nil
}

type e2eTestSystem struct {
	router      http.Handler
	userRepo    repository.UserRepository
	productRepo repository.ProductRepository
	orderRepo   repository.OrderRepository
	outboxRepo  repository.OutboxRepository
	broker      *inMemoryEventBroker
}

func setupE2ESystem() *e2eTestSystem {
	userRepo := repository.NewUserMemoryRepository()
	productRepo := repository.NewProductMemoryRepository()
	orderRepo := repository.NewOrderMemoryRepository()
	outboxRepo := repository.NewOutboxMemoryRepository()
	paymentRepo := repository.NewPaymentMemoryRepository()
	idempotencyRepo := repository.NewIdempotencyMemoryRepository()
	txManager := repository.NewMemoryTxManager()
	broker := &inMemoryEventBroker{}

	userSvc := service.NewUserService(userRepo)
	productSvc := service.NewProductService(productRepo)
	orderSvc := service.NewOrderService(orderRepo, userRepo, productRepo, txManager, outboxRepo)
	paymentSvc := service.NewPaymentService(paymentRepo, orderRepo, txManager, idempotencyRepo)

	router := handler.NewRouter(userSvc, productSvc, orderSvc, paymentSvc)

	return &e2eTestSystem{
		router:      router,
		userRepo:    userRepo,
		productRepo: productRepo,
		orderRepo:   orderRepo,
		outboxRepo:  outboxRepo,
		broker:      broker,
	}
}

type mockSagaPaymentClient struct {
	mu         sync.Mutex
	refundDone bool
}

func (m *mockSagaPaymentClient) ReservePayment(ctx context.Context, orderID uuid.UUID, amount float64) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (m *mockSagaPaymentClient) RefundPayment(ctx context.Context, paymentID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refundDone = true
	return nil
}

type mockSagaDeliveryClient struct {
	mu         sync.Mutex
	cancelDone bool
	shouldFail bool
}

func (m *mockSagaDeliveryClient) CreateDelivery(ctx context.Context, orderID uuid.UUID) (uuid.UUID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shouldFail {
		return uuid.Nil, errors.New("delivery courier unavailable")
	}
	return uuid.New(), nil
}

func (m *mockSagaDeliveryClient) CancelDelivery(ctx context.Context, deliveryID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelDone = true
	return nil
}

func TestE2E_OrderProcessing_HappyPath(t *testing.T) {
	sys := setupE2ESystem()
	ctx := context.Background()

	// 1. Creating user and product
	user, _ := domain.NewUser("Frank", "frank@example.com")
	_ = sys.userRepo.Save(ctx, user)

	product, _ := domain.NewProduct("Mechanical Gaming Mouse", 79.99)
	_ = sys.productRepo.Save(ctx, product)

	// 2. Client sends HTTP POST /orders
	orderBody := map[string]any{
		"user_id": user.ID.String(),
		"items": []map[string]any{
			{"product_id": product.ID.String(), "quantity": 1},
		},
	}
	bodyBytes, _ := json.Marshal(orderBody)
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	sys.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("step 2 failed: expected 201 Created, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var createdOrder domain.Order
	_ = json.Unmarshal(rec.Body.Bytes(), &createdOrder)

	if createdOrder.Status != domain.OrderStatusPending {
		t.Errorf("expected initial order status 'pending', got '%s'", createdOrder.Status)
	}

	// 3. Check if Outbox has the record with PENDING status
	pendingOutbox, err := sys.outboxRepo.FetchPending(ctx, 10)
	if err != nil {
		t.Fatalf("step 3 failed: fetch outbox error: %v", err)
	}
	if len(pendingOutbox) != 1 {
		t.Fatalf("expected 1 pending outbox record, got %d", len(pendingOutbox))
	}
	if pendingOutbox[0].AggregateID != createdOrder.ID {
		t.Errorf("outbox aggregate ID mismatch: expected %s, got %s", createdOrder.ID, pendingOutbox[0].AggregateID)
	}

	// 4. Launching background Outbox Worker: it is reading PENDING and sendind to the broker
	outboxWorker := worker.NewOutboxWorker(sys.outboxRepo, sys.broker, 20*time.Millisecond, 10)
	workerCtx, cancelWorker := context.WithCancel(ctx)

	go outboxWorker.Start(workerCtx)
	time.Sleep(80 * time.Millisecond) // give broker time to execute
	cancelWorker()
	outboxWorker.Wait()

	// Check if broker got the message and outbox cleared up from pending
	sys.broker.mu.Lock()
	messagesCount := len(sys.broker.messages)
	sys.broker.mu.Unlock()

	if messagesCount != 1 {
		t.Fatalf("expected 1 event published to broker, got %d", messagesCount)
	}

	leftPending, _ := sys.outboxRepo.FetchPending(ctx, 10)
	if len(leftPending) != 0 {
		t.Errorf("expected 0 pending outbox records after worker processing, got %d", len(leftPending))
	}

	// 5. Executing OrderCreated event by Saga orchestrator: Payment -> Delivery -> Finalization
	paymentClient := &mockSagaPaymentClient{}
	deliveryClient := &mockSagaDeliveryClient{shouldFail: false}

	orderSaga := saga.CreateOrderSaga(&createdOrder, sys.orderRepo, paymentClient, deliveryClient)
	if err := orderSaga.Execute(ctx); err != nil {
		t.Fatalf("step 5 failed: saga execution error: %v", err)
	}

	// 6. Checking order final status in DB: status must be completed
	finalOrder, err := sys.orderRepo.GetByID(ctx, createdOrder.ID)
	if err != nil {
		t.Fatalf("failed to get final order: %v", err)
	}

	if finalOrder.Status != domain.OrderStatusCompleted {
		t.Errorf("expected final order status 'completed', got '%s'", finalOrder.Status)
	}
	if paymentClient.refundDone {
		t.Errorf("payment should NOT be refunded on happy path")
	}
}

func TestE2E_OrderProcessing_DeliveryFailure_Compensates(t *testing.T) {
	sys := setupE2ESystem()
	ctx := context.Background()

	order := domain.NewOrder(uuid.New())
	_ = sys.orderRepo.Save(ctx, order)

	paymentClient := &mockSagaPaymentClient{}
	deliveryClient := &mockSagaDeliveryClient{shouldFail: true} // симулируем сбой доставки

	orderSaga := saga.CreateOrderSaga(&order, sys.orderRepo, paymentClient, deliveryClient)
	err := orderSaga.Execute(ctx)
	if err == nil {
		t.Fatal("expected saga to return error when delivery fails")
	}

	_ = order.Cancel()
	_ = sys.orderRepo.Save(ctx, order)

	// Verifying that the refund has been processed
	paymentClient.mu.Lock()
	refunded := paymentClient.refundDone
	paymentClient.mu.Unlock()

	if !refunded {
		t.Errorf("expected payment refund compensation to be executed")
	}

	// Verifying that the order has been moved to the "cancelled" status.
	savedOrder, _ := sys.orderRepo.GetByID(ctx, order.ID)
	if savedOrder.Status != domain.OrderStatusCancelled {
		t.Errorf("expected order status 'cancelled' after compensation, got '%s'", savedOrder.Status)
	}
}
