package service_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/google/uuid"
)

func setupPaymentTest() (*service.PaymentService, *service.OrderService, *repository.OrderMemoryRepository, *repository.PaymentMemoryRepository, *repository.IdempotencyMemoryRepository) {
	orderRepo := repository.NewOrderMemoryRepository()
	paymentRepo := repository.NewPaymentMemoryRepository()
	userRepo := repository.NewUserMemoryRepository()
	productRepo := repository.NewProductMemoryRepository()
	idempotencyRepo := repository.NewIdempotencyMemoryRepository()
	txManager := repository.NewMemoryTxManager()

	orderSvc := service.NewOrderService(orderRepo, userRepo, productRepo, txManager, nil)
	paymentSvc := service.NewPaymentService(paymentRepo, orderRepo, txManager, idempotencyRepo)

	return paymentSvc, orderSvc, orderRepo, paymentRepo, idempotencyRepo
}

func createTestOrder(t *testing.T, orderRepo *repository.OrderMemoryRepository) domain.Order {
	t.Helper()
	order := domain.NewOrder(uuid.New())
	order.AddItem(domain.Product{
		ID:    uuid.New(),
		Name:  "Test Game",
		Price: 49.99,
	}, 1)
	if err := orderRepo.Save(context.Background(), order); err != nil {
		t.Fatalf("failed to save test order: %v", err)
	}
	return order
}

func TestPaymentService_Success(t *testing.T) {
	paymentSvc, _, orderRepo, paymentRepo, _ := setupPaymentTest()
	order := createTestOrder(t, orderRepo)

	payment, err := paymentSvc.ProcessPayment(context.Background(), order.ID, "idemp-1")
	if err != nil {
		t.Fatalf("expected payment success, got %v", err)
	}

	if payment.Status != domain.PaymentStatusSuccess {
		t.Errorf("expected payment status success, got %s", payment.Status)
	}

	if payment.Amount != 49.99 {
		t.Errorf("expected amount 49.99, got %.2f", payment.Amount)
	}

	// Verify order is paid
	updatedOrder, err := orderRepo.GetByID(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}
	if updatedOrder.Status != domain.OrderStatusPaid {
		t.Errorf("expected order status paid, got %s", updatedOrder.Status)
	}

	// Verify payment exists in repository
	savedPayment, err := paymentRepo.GetByID(context.Background(), payment.ID)
	if err != nil {
		t.Fatalf("failed to get saved payment: %v", err)
	}
	if savedPayment.ID != payment.ID {
		t.Errorf("expected saved payment ID %s, got %s", payment.ID, savedPayment.ID)
	}
}

func TestPaymentService_IdempotencyKey(t *testing.T) {
	paymentSvc, _, orderRepo, _, _ := setupPaymentTest()
	order := createTestOrder(t, orderRepo)

	key := "test-idemp-unique-123"

	// 1. Initial call
	p1, err := paymentSvc.ProcessPayment(context.Background(), order.ID, key)
	if err != nil {
		t.Fatalf("first payment failed: %v", err)
	}

	// 2. Second call with exact same key -> must return identical payment, no errors
	p2, err := paymentSvc.ProcessPayment(context.Background(), order.ID, key)
	if err != nil {
		t.Fatalf("replayed payment failed: %v", err)
	}

	if p1.ID != p2.ID {
		t.Errorf("expected identical payment IDs, got %s and %s", p1.ID, p2.ID)
	}
	if p1.Amount != p2.Amount {
		t.Errorf("expected identical amounts, got %.2f and %.2f", p1.Amount, p2.Amount)
	}

	// 3. Third call with different key on already paid order -> must fail with ErrOrderAlreadyPaid
	_, err = paymentSvc.ProcessPayment(context.Background(), order.ID, "different-key-456")
	if !errors.Is(err, domain.ErrOrderAlreadyPaid) {
		t.Errorf("expected ErrOrderAlreadyPaid, got %v", err)
	}
}

func TestPaymentService_OrderAlreadyCancelled(t *testing.T) {
	paymentSvc, _, orderRepo, _, _ := setupPaymentTest()
	order := createTestOrder(t, orderRepo)

	_ = order.Cancel()
	_ = orderRepo.Save(context.Background(), order)

	_, err := paymentSvc.ProcessPayment(context.Background(), order.ID, "")
	if !errors.Is(err, domain.ErrOrderAlreadyCancelled) {
		t.Errorf("expected ErrOrderAlreadyCancelled, got %v", err)
	}
}

func TestPaymentService_EmptyOrder(t *testing.T) {
	paymentSvc, _, orderRepo, _, _ := setupPaymentTest()
	emptyOrder := domain.NewOrder(uuid.New())
	_ = orderRepo.Save(context.Background(), emptyOrder)

	_, err := paymentSvc.ProcessPayment(context.Background(), emptyOrder.ID, "")
	if !errors.Is(err, domain.ErrEmptyOrder) {
		t.Errorf("expected ErrEmptyOrder, got %v", err)
	}
}

func TestPaymentService_ConcurrentPayments_DifferentKeys(t *testing.T) {
	paymentSvc, _, orderRepo, _, _ := setupPaymentTest()
	order := createTestOrder(t, orderRepo)

	concurrency := 20
	var successCount int32
	var failCount int32
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			uniqueKey := uuid.New().String()
			_, err := paymentSvc.ProcessPayment(context.Background(), order.ID, uniqueKey)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else if errors.Is(err, domain.ErrOrderAlreadyPaid) {
				atomic.AddInt32(&failCount, 1)
			}
		}()
	}

	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected exactly 1 successful payment, got %d", successCount)
	}
	if failCount != int32(concurrency-1) {
		t.Errorf("expected %d failed payments with ErrOrderAlreadyPaid, got %d", concurrency-1, failCount)
	}

	updatedOrder, _ := orderRepo.GetByID(context.Background(), order.ID)
	if updatedOrder.Status != domain.OrderStatusPaid {
		t.Errorf("expected final status paid, got %s", updatedOrder.Status)
	}
}

func TestPaymentService_ConcurrentPayments_SameKey(t *testing.T) {
	paymentSvc, _, orderRepo, _, _ := setupPaymentTest()
	order := createTestOrder(t, orderRepo)

	concurrency := 20
	sharedKey := "shared-idempotency-key"
	payments := make([]domain.Payment, concurrency)
	errorsList := make([]error, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := paymentSvc.ProcessPayment(context.Background(), order.ID, sharedKey)
			payments[idx] = p
			errorsList[idx] = err
		}()
	}

	wg.Wait()

	var firstID uuid.UUID
	for i := 0; i < concurrency; i++ {
		if errorsList[i] != nil {
			t.Errorf("goroutine %d failed: %v", i, errorsList[i])
			continue
		}
		if firstID == uuid.Nil {
			firstID = payments[i].ID
		} else if payments[i].ID != firstID {
			t.Errorf("goroutine %d got different payment ID: %s vs %s", i, payments[i].ID, firstID)
		}
	}
}

func TestPaymentService_CancelVsPaymentRace(t *testing.T) {
	paymentSvc, orderSvc, orderRepo, _, _ := setupPaymentTest()
	order := createTestOrder(t, orderRepo)

	var wg sync.WaitGroup
	wg.Add(2)

	var payErr, cancelErr error

	go func() {
		defer wg.Done()
		_, payErr = paymentSvc.ProcessPayment(context.Background(), order.ID, uuid.New().String())
	}()

	go func() {
		defer wg.Done()
		_, cancelErr = orderSvc.CancelOrder(context.Background(), order.ID)
	}()

	wg.Wait()

	finalOrder, _ := orderRepo.GetByID(context.Background(), order.ID)

	// Exactly one operation must succeed, and the other must fail with conflict
	if payErr == nil && cancelErr == nil {
		t.Fatalf("both payment and cancellation succeeded! Final status: %s", finalOrder.Status)
	}

	if payErr == nil {
		if finalOrder.Status != domain.OrderStatusPaid {
			t.Errorf("payment succeeded but status is %s", finalOrder.Status)
		}
		if !errors.Is(cancelErr, domain.ErrOrderAlreadyPaid) {
			t.Errorf("expected cancelErr to be ErrOrderAlreadyPaid, got %v", cancelErr)
		}
	} else {
		if finalOrder.Status != domain.OrderStatusCancelled {
			t.Errorf("cancel succeeded but status is %s", finalOrder.Status)
		}
		if !errors.Is(payErr, domain.ErrOrderAlreadyCancelled) {
			t.Errorf("expected payErr to be ErrOrderAlreadyCancelled, got %v", payErr)
		}
	}
}

func TestOptimisticLocking(t *testing.T) {
	_, _, orderRepo, _, _ := setupPaymentTest()
	order := createTestOrder(t, orderRepo)

	// Read order by process 1 and process 2
	orderP1, err := orderRepo.GetByID(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("process 1 failed to get order: %v", err)
	}

	orderP2, err := orderRepo.GetByID(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("process 2 failed to get order: %v", err)
	}

	// Process 1 updates order successfully
	_ = orderP1.Pay()
	if err := orderRepo.SaveOptimistic(context.Background(), orderP1); err != nil {
		t.Fatalf("process 1 failed to save optimistically: %v", err)
	}

	// Process 2 tries to update with stale version
	_ = orderP2.Cancel()
	err = orderRepo.SaveOptimistic(context.Background(), orderP2)
	if !errors.Is(err, domain.ErrOptimisticLockConflict) {
		t.Fatalf("expected ErrOptimisticLockConflict for stale version update, got %v", err)
	}
}
