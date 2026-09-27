package saga

import (
	"context"
	"fmt"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/google/uuid"
)

type PaymentClient interface {
	ReservePayment(ctx context.Context, orderID uuid.UUID, amount float64) (uuid.UUID, error)
	RefundPayment(ctx context.Context, paymentID uuid.UUID) error
}

type DeliveryClient interface {
	CreateDelivery(ctx context.Context, orderID uuid.UUID) (uuid.UUID, error)
	CancelDelivery(ctx context.Context, deliveryID uuid.UUID) error
}

// 1. Reserve Payment
type PaymentStep struct {
	client    PaymentClient
	order     *domain.Order
	paymentID uuid.UUID
}

func NewPaymentStep(client PaymentClient, order *domain.Order) *PaymentStep {
	return &PaymentStep{client: client, order: order}
}

func (s *PaymentStep) Name() string { return "ReservePayment" }

func (s *PaymentStep) Execute(ctx context.Context) error {
	paymentID, err := s.client.ReservePayment(ctx, s.order.ID, s.order.CalculateTotal())
	if err != nil {
		return fmt.Errorf("failed to reserve payment: %w", err)
	}
	s.paymentID = paymentID
	return nil
}

func (s *PaymentStep) Compensate(ctx context.Context) error {
	if s.paymentID == uuid.Nil {
		return nil
	}
	return s.client.RefundPayment(ctx, s.paymentID)
}

// 2. Create Delivery
type DeliveryStep struct {
	client     DeliveryClient
	order      *domain.Order
	deliveryID uuid.UUID
}

func NewDeliveryStep(client DeliveryClient, order *domain.Order) *DeliveryStep {
	return &DeliveryStep{client: client, order: order}
}

func (s *DeliveryStep) Name() string { return "CreateDelivery" }

func (s *DeliveryStep) Execute(ctx context.Context) error {
	deliveryID, err := s.client.CreateDelivery(ctx, s.order.ID)
	if err != nil {
		return fmt.Errorf("failed to create delivery: %w", err)
	}
	s.deliveryID = deliveryID
	return nil
}

func (s *DeliveryStep) Compensate(ctx context.Context) error {
	if s.deliveryID == uuid.Nil {
		return nil
	}
	return s.client.CancelDelivery(ctx, s.deliveryID)
}

// 3. Complete Order
type OrderCompletionStep struct {
	orderRepo repository.OrderRepository
	order     *domain.Order
}

func NewOrderCompletionStep(orderRepo repository.OrderRepository, order *domain.Order) *OrderCompletionStep {
	return &OrderCompletionStep{orderRepo: orderRepo, order: order}
}

func (s *OrderCompletionStep) Name() string { return "CompleteOrder" }

func (s *OrderCompletionStep) Execute(ctx context.Context) error {
	if err := s.order.Complete(); err != nil {
		return err
	}
	return s.orderRepo.Save(ctx, *s.order)
}

func (s *OrderCompletionStep) Compensate(ctx context.Context) error {
	_ = s.order.Cancel()
	return s.orderRepo.Save(ctx, *s.order)
}

func CreateOrderSaga(
	order *domain.Order,
	orderRepo repository.OrderRepository,
	paymentClient PaymentClient,
	deliveryClient DeliveryClient,
) *Orchestrator {
	orchestrator := NewOrchestrator()

	orchestrator.
		AddStep(NewPaymentStep(paymentClient, order)).
		AddStep(NewDeliveryStep(deliveryClient, order)).
		AddStep(NewOrderCompletionStep(orderRepo, order))

	return orchestrator
}
