package domain_test

import (
	"errors"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/google/uuid"
)

func TestOrder_AddItemAndCalculateTotal(t *testing.T) {
	userID := uuid.New()
	order := domain.NewOrder(userID)

	p1, err := domain.NewProduct("Go Book", 50.0)
	if err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	p2, err := domain.NewProduct("Sticker", 5.0)
	if err != nil {
		t.Fatalf("failed to create product: %v", err)
	}

	order.AddItem(p1, 2)
	order.AddItem(p2, 3)

	expectedTotal := 115.0
	if total := order.CalculateTotal(); total != expectedTotal {
		t.Errorf("expected total %.2f, got %.2f", expectedTotal, total)
	}

	// Verifying the increase in the quantity of an existing product
	order.AddItem(p2, 2)
	expectedTotal = 125.0
	if total := order.CalculateTotal(); total != expectedTotal {
		t.Errorf("expected total %.2f, got %.2f", expectedTotal, total)
	}

	// Delete the item
	if err := order.RemoveItem(p1.ID); err != nil {
		t.Fatalf("unexpected error removing item: %v", err)
	}

	expectedTotal = 25.0
	if total := order.CalculateTotal(); total != expectedTotal {
		t.Errorf("expected total %.2f after removal, got %.2f", expectedTotal, total)
	}
}

func TestOrder_PayAndCancelTransitions(t *testing.T) {
	t.Run("cannot pay empty order", func(t *testing.T) {
		order := domain.NewOrder(uuid.New())
		err := order.Pay()
		if !errors.Is(err, domain.ErrEmptyOrder) {
			t.Errorf("expected ErrEmptyOrder, got %v", err)
		}
	})

	t.Run("successful pay and cannot cancel paid order", func(t *testing.T) {
		order := domain.NewOrder(uuid.New())
		p, _ := domain.NewProduct("Course", 100.0)
		order.AddItem(p, 1)

		if err := order.Pay(); err != nil {
			t.Fatalf("expected order to be paid, got %v", err)
		}

		if order.Status != domain.OrderStatusPaid {
			t.Errorf("expected status %s, got %s", domain.OrderStatusPaid, order.Status)
		}

		// Attempt to cancel a payment that has already been made
		err := order.Cancel()
		if !errors.Is(err, domain.ErrOrderAlreadyPaid) {
			t.Errorf("expected ErrOrderAlreadyPaid, got %v", err)
		}
	})
}
