package saga_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/saga"
	"github.com/google/uuid"
)

type mockPaymentClient struct {
	reserveErr error
	refundDone bool
	paymentID  uuid.UUID
}

func (m *mockPaymentClient) ReservePayment(ctx context.Context, orderID uuid.UUID, amount float64) (uuid.UUID, error) {
	if m.reserveErr != nil {
		return uuid.Nil, m.reserveErr
	}
	m.paymentID = uuid.New()
	return m.paymentID, nil
}

func (m *mockPaymentClient) RefundPayment(ctx context.Context, paymentID uuid.UUID) error {
	m.refundDone = true
	return nil
}

type mockDeliveryClient struct {
	deliveryErr error
	cancelDone  bool
	deliveryID  uuid.UUID
}

func (m *mockDeliveryClient) CreateDelivery(ctx context.Context, orderID uuid.UUID) (uuid.UUID, error) {
	if m.deliveryErr != nil {
		return uuid.Nil, m.deliveryErr
	}
	m.deliveryID = uuid.New()
	return m.deliveryID, nil
}

func (m *mockDeliveryClient) CancelDelivery(ctx context.Context, deliveryID uuid.UUID) error {
	m.cancelDone = true
	return nil
}

func TestOrderSaga_SuccessFlow(t *testing.T) {
	orderRepo := repository.NewOrderMemoryRepository()
	order := domain.NewOrder(uuid.New())
	product, _ := domain.NewProduct("Product 1", 100.0)
	order.AddItem(product, 1)
	_ = orderRepo.Save(context.Background(), order)

	paymentClient := &mockPaymentClient{}
	deliveryClient := &mockDeliveryClient{}

	orchestrator := saga.CreateOrderSaga(&order, orderRepo, paymentClient, deliveryClient)

	err := orchestrator.Execute(context.Background())
	if err != nil {
		t.Fatalf("expected saga to succeed, got %v", err)
	}

	savedOrder, _ := orderRepo.GetByID(context.Background(), order.ID)
	if savedOrder.Status != domain.OrderStatusCompleted {
		t.Errorf("expected order status %s, got %s", domain.OrderStatusCompleted, savedOrder.Status)
	}

	if paymentClient.refundDone {
		t.Errorf("refund should not be called on success")
	}
	if deliveryClient.cancelDone {
		t.Errorf("delivery cancel should not be called on success")
	}
}

func TestOrderSaga_PaymentFailure_Compensates(t *testing.T) {
	orderRepo := repository.NewOrderMemoryRepository()
	order := domain.NewOrder(uuid.New())
	_ = orderRepo.Save(context.Background(), order)

	paymentClient := &mockPaymentClient{reserveErr: errors.New("insufficient funds")}
	deliveryClient := &mockDeliveryClient{}

	orchestrator := saga.CreateOrderSaga(&order, orderRepo, paymentClient, deliveryClient)

	err := orchestrator.Execute(context.Background())
	if err == nil {
		t.Fatal("expected saga error, got nil")
	}

	if deliveryClient.deliveryID != uuid.Nil {
		t.Errorf("delivery should not be initiated when payment fails")
	}
}

func TestOrderSaga_DeliveryFailure_CompensatesPayment(t *testing.T) {
	orderRepo := repository.NewOrderMemoryRepository()
	order := domain.NewOrder(uuid.New())
	_ = orderRepo.Save(context.Background(), order)

	paymentClient := &mockPaymentClient{}
	deliveryClient := &mockDeliveryClient{deliveryErr: errors.New("delivery warehouse closed")}

	orchestrator := saga.CreateOrderSaga(&order, orderRepo, paymentClient, deliveryClient)

	err := orchestrator.Execute(context.Background())
	if err == nil {
		t.Fatal("expected saga error, got nil")
	}

	if !paymentClient.refundDone {
		t.Errorf("expected payment refund compensation to be executed")
	}
}
