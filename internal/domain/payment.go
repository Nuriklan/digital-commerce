package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type PaymentStatus string

const (
	PaymentStatusPending PaymentStatus = "pending"
	PaymentStatusSuccess PaymentStatus = "success"
	PaymentStatusFailed  PaymentStatus = "failed"
)

var (
	ErrInvalidAmount    = errors.New("payment amount must be greater than 0")
	ErrPaymentNotFound  = errors.New("payment not found")
	ErrAlreadyProcessed = errors.New("payment already processed")
)

type Payment struct {
	ID        uuid.UUID
	OrderID   uuid.UUID
	Amount    float64
	Status    PaymentStatus
	CreatedAt time.Time
}

func NewPayment(orderID uuid.UUID, amount float64) (Payment, error) {
	if amount <= 0 {
		return Payment{}, ErrInvalidAmount
	}

	return Payment{
		ID:        uuid.New(),
		OrderID:   orderID,
		Amount:    amount,
		Status:    PaymentStatusPending,
		CreatedAt: time.Now(),
	}, nil
}

func (p *Payment) MarkSuccess() error {
	if p.Status != PaymentStatusPending {
		return ErrAlreadyProcessed
	}
	p.Status = PaymentStatusSuccess
	return nil
}

func (p *Payment) MarkFailed() error {
	if p.Status != PaymentStatusPending {
		return ErrAlreadyProcessed
	}
	p.Status = PaymentStatusFailed
	return nil
}
