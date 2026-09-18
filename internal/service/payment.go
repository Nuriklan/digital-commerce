package service

import (
	"fmt"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/google/uuid"
)

type PaymentService struct {
	paymentRepo repository.PaymentRepository
	orderRepo   repository.OrderRepository
}

func NewPaymentService(
	paymentRepo repository.PaymentRepository,
	orderRepo repository.OrderRepository,
) *PaymentService {
	return &PaymentService{
		paymentRepo: paymentRepo,
		orderRepo:   orderRepo,
	}
}

func (s *PaymentService) ProcessPayment(orderID uuid.UUID) (domain.Payment, error) {
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return domain.Payment{}, fmt.Errorf("order not found: %w", err)
	}

	total := order.CalculateTotal()
	payment, err := domain.NewPayment(order.ID, total)
	if err != nil {
		return domain.Payment{}, err
	}

	if err := order.Pay(); err != nil {
		_ = payment.MarkFailed()
		_ = s.paymentRepo.Save(payment)
		return domain.Payment{}, fmt.Errorf("payment failed: %w", err)
	}

	_ = payment.MarkSuccess()

	if err := s.orderRepo.Save(order); err != nil {
		return domain.Payment{}, fmt.Errorf("failed to update order: %w", err)
	}

	if err := s.paymentRepo.Save(payment); err != nil {
		return domain.Payment{}, fmt.Errorf("failed to save payment: %w", err)
	}

	return payment, nil
}

func (s *PaymentService) GetPayment(id uuid.UUID) (domain.Payment, error) {
	return s.paymentRepo.GetByID(id)
}
