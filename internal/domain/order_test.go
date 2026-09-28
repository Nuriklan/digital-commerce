package domain_test

import (
	"errors"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/google/uuid"
)

// mustCreateProduct is a test helper for creating a valid product.
// Calling t.Helper() is crucial: if creation fails, Go will point to the line in the test
// where mustCreateProduct was called, rather than the line inside the helper itself.
func mustCreateProduct(t *testing.T, name string, price float64) domain.Product {
	t.Helper()
	p, err := domain.NewProduct(name, price)
	if err != nil {
		t.Fatalf("test helper mustCreateProduct failed: %v", err)
	}
	return p
}

// TestOrder_AddItemAndSubtotal verifies the addition of items and the aggregation of amounts.
func TestOrder_AddItemAndSubtotal(t *testing.T) {
	order := domain.NewOrder(uuid.New())
	p1 := mustCreateProduct(t, "Go Book", 50.0)
	p2 := mustCreateProduct(t, "Sticker", 5.0)

	// Adding various products
	order.AddItem(p1, 2)
	order.AddItem(p2, 3)

	expectedTotal := 115.0
	if total := order.CalculateTotal(); total != expectedTotal {
		t.Errorf("expected total %.2f, got %.2f", expectedTotal, total)
	}

	// Adding the same item again should increase the quantity rather than creating duplicates
	order.AddItem(p2, 2)
	if len(order.Items) != 2 {
		t.Fatalf("expected 2 distinct items, got %d", len(order.Items))
	}

	expectedTotal = 125.0
	if total := order.CalculateTotal(); total != expectedTotal {
		t.Errorf("expected total %.2f after quantity increment, got %.2f", expectedTotal, total)
	}

	// Deleting an existing product
	if err := order.RemoveItem(p1.ID); err != nil {
		t.Fatalf("unexpected error removing item: %v", err)
	}
	if len(order.Items) != 1 {
		t.Fatalf("expected 1 item after removal, got %d", len(order.Items))
	}

	// Deleting a non-existent item should return domain.ErrItemNotFound
	if err := order.RemoveItem(uuid.New()); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("expected ErrItemNotFound, got %v", err)
	}
}

// TestOrder_StatusTransitions uses the idiomatic table-driven test pattern
// to verify the order status state machine.
func TestOrder_StatusTransitions(t *testing.T) {
	tests := []struct {
		name          string
		setupOrder    func(t *testing.T) domain.Order
		action        func(o *domain.Order) error
		expectedError error
		expectedState domain.OrderStatus
	}{
		{
			name: "cannot pay empty order",
			setupOrder: func(t *testing.T) domain.Order {
				return domain.NewOrder(uuid.New())
			},
			action: func(o *domain.Order) error {
				return o.Pay()
			},
			expectedError: domain.ErrEmptyOrder,
			expectedState: domain.OrderStatusPending,
		},
		{
			name: "successful payment of order with items",
			setupOrder: func(t *testing.T) domain.Order {
				o := domain.NewOrder(uuid.New())
				p := mustCreateProduct(t, "Course", 100.0)
				o.AddItem(p, 1)
				return o
			},
			action: func(o *domain.Order) error {
				return o.Pay()
			},
			expectedError: nil,
			expectedState: domain.OrderStatusPaid,
		},
		{
			name: "already paid order cannot be paid again",
			setupOrder: func(t *testing.T) domain.Order {
				o := domain.NewOrder(uuid.New())
				p := mustCreateProduct(t, "Course", 100.0)
				o.AddItem(p, 1)
				_ = o.Pay()
				return o
			},
			action: func(o *domain.Order) error {
				return o.Pay()
			},
			expectedError: domain.ErrOrderAlreadyPaid,
			expectedState: domain.OrderStatusPaid,
		},
		{
			name: "already paid order cannot be cancelled directly",
			setupOrder: func(t *testing.T) domain.Order {
				o := domain.NewOrder(uuid.New())
				p := mustCreateProduct(t, "Course", 100.0)
				o.AddItem(p, 1)
				_ = o.Pay()
				return o
			},
			action: func(o *domain.Order) error {
				return o.Cancel()
			},
			expectedError: domain.ErrOrderAlreadyPaid,
			expectedState: domain.OrderStatusPaid,
		},
		{
			name: "pending order can be cancelled",
			setupOrder: func(t *testing.T) domain.Order {
				return domain.NewOrder(uuid.New())
			},
			action: func(o *domain.Order) error {
				return o.Cancel()
			},
			expectedError: nil,
			expectedState: domain.OrderStatusCancelled,
		},
		{
			name: "cancelled order cannot be paid",
			setupOrder: func(t *testing.T) domain.Order {
				o := domain.NewOrder(uuid.New())
				_ = o.Cancel()
				return o
			},
			action: func(o *domain.Order) error {
				return o.Pay()
			},
			expectedError: domain.ErrOrderAlreadyCancelled,
			expectedState: domain.OrderStatusCancelled,
		},
		{
			name: "cancelled order cannot be completed",
			setupOrder: func(t *testing.T) domain.Order {
				o := domain.NewOrder(uuid.New())
				_ = o.Cancel()
				return o
			},
			action: func(o *domain.Order) error {
				return o.Complete()
			},
			expectedError: domain.ErrOrderAlreadyCancelled,
			expectedState: domain.OrderStatusCancelled,
		},
		{
			name: "paid order can be completed",
			setupOrder: func(t *testing.T) domain.Order {
				o := domain.NewOrder(uuid.New())
				p := mustCreateProduct(t, "License", 200.0)
				o.AddItem(p, 1)
				_ = o.Pay()
				return o
			},
			action: func(o *domain.Order) error {
				return o.Complete()
			},
			expectedError: nil,
			expectedState: domain.OrderStatusCompleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := tt.setupOrder(t)

			err := tt.action(&order)

			// Checking the returned error
			if tt.expectedError != nil {
				if !errors.Is(err, tt.expectedError) {
					t.Errorf("expected error %v, got %v", tt.expectedError, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verification of the correctness of the resulting status
			if order.Status != tt.expectedState {
				t.Errorf("expected status %s, got %s", tt.expectedState, order.Status)
			}
		})
	}
}
