package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/google/uuid"
)

type PaymentService struct {
	paymentRepo     repository.PaymentRepository
	orderRepo       repository.OrderRepository
	txManager       repository.TxManager
	idempotencyRepo repository.IdempotencyRepository
}

func NewPaymentService(
	paymentRepo repository.PaymentRepository,
	orderRepo repository.OrderRepository,
	txManager repository.TxManager,
	idempotencyRepo repository.IdempotencyRepository,
) *PaymentService {
	return &PaymentService{
		paymentRepo:     paymentRepo,
		orderRepo:       orderRepo,
		txManager:       txManager,
		idempotencyRepo: idempotencyRepo,
	}
}

// ProcessPayment executes the payment process under transaction boundaries,
// pessimistic locking, and idempotency key deduplication.
func (s *PaymentService) ProcessPayment(ctx context.Context, orderID uuid.UUID, idempotencyKey string) (domain.Payment, error) {
	// 1. Fast path: check idempotency cache before starting a transaction
	if idempotencyKey != "" && s.idempotencyRepo != nil {
		existing, err := s.idempotencyRepo.Get(ctx, idempotencyKey)
		if err != nil {
			return domain.Payment{}, fmt.Errorf("failed to check idempotency key: %w", err)
		}
		if existing != nil {
			var payment domain.Payment
			if err := json.Unmarshal(existing.ResponseBody, &payment); err == nil {
				return payment, nil
			}
		}
	}

	var payment domain.Payment

	// 2. Execute transactional workflow
	err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		// Concurrency check: verify if another transaction just committed this idempotency key
		if idempotencyKey != "" && s.idempotencyRepo != nil {
			existing, err := s.idempotencyRepo.Get(txCtx, idempotencyKey)
			if err != nil {
				return fmt.Errorf("failed to check idempotency key within tx: %w", err)
			}
			if existing != nil {
				if err := json.Unmarshal(existing.ResponseBody, &payment); err == nil {
					return nil
				}
			}
		}

		// Step A: Pessimistic Lock - Lock order row with SELECT ... FOR UPDATE
		order, err := s.orderRepo.GetByIDForUpdate(txCtx, orderID)
		if err != nil {
			return fmt.Errorf("order not found: %w", err)
		}

		// Step B: Validate business invariants
		if order.Status == domain.OrderStatusPaid {
			return domain.ErrOrderAlreadyPaid
		}
		if order.Status == domain.OrderStatusCancelled {
			return domain.ErrOrderAlreadyCancelled
		}
		if len(order.Items) == 0 {
			return domain.ErrEmptyOrder
		}

		// Step C: Create payment entity (Pending status)
		total := order.CalculateTotal()
		newPayment, err := domain.NewPayment(order.ID, total)
		if err != nil {
			return err
		}
		if err := s.paymentRepo.Save(txCtx, newPayment); err != nil {
			return fmt.Errorf("failed to save initial pending payment: %w", err)
		}

		// Step D: Reserve order (transition status to Paid)
		if err := order.Pay(); err != nil {
			_ = newPayment.MarkFailed()
			_ = s.paymentRepo.Save(txCtx, newPayment)
			return err
		}

		// Step E: Update payment status to Success
		if err := newPayment.MarkSuccess(); err != nil {
			return err
		}
		if err := s.paymentRepo.Save(txCtx, newPayment); err != nil {
			return fmt.Errorf("failed to update payment to success: %w", err)
		}

		// Step F: Update order status in database
		if err := s.orderRepo.Save(txCtx, order); err != nil {
			return fmt.Errorf("failed to update order: %w", err)
		}

		// Step G: Store idempotency key record for deduplication
		if idempotencyKey != "" && s.idempotencyRepo != nil {
			bodyBytes, err := json.Marshal(newPayment)
			if err != nil {
				return fmt.Errorf("failed to serialize payment for idempotency: %w", err)
			}
			record := domain.IdempotencyRecord{
				Key:          idempotencyKey,
				PaymentID:    newPayment.ID,
				OrderID:      order.ID,
				StatusCode:   http.StatusCreated,
				ResponseBody: bodyBytes,
				CreatedAt:    time.Now(),
			}
			if err := s.idempotencyRepo.Save(txCtx, record); err != nil {
				return fmt.Errorf("failed to save idempotency key: %w", err)
			}
		}

		payment = newPayment
		return nil
	})

	if err != nil {
		return domain.Payment{}, err
	}

	return payment, nil
}

func (s *PaymentService) GetPayment(ctx context.Context, id uuid.UUID) (domain.Payment, error) {
	return s.paymentRepo.GetByID(ctx, id)
}
