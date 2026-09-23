package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusCancelled OrderStatus = "cancelled"
)

var (
	ErrOrderAlreadyPaid      = errors.New("order is already paid")
	ErrOrderAlreadyCancelled = errors.New("order is already cancelled")
	ErrEmptyOrder            = errors.New("order has no items")
	ErrItemNotFound          = errors.New("item not found in order")
	ErrOrderNotFound          = errors.New("order not found")
	ErrOptimisticLockConflict = errors.New("optimistic lock conflict: order was updated concurrently")
)

type OrderItem struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	Name      string
	Price     float64
	Quantity  int
}

func (i OrderItem) Subtotal() float64 {
	return i.Price * float64(i.Quantity)
}

type Order struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Items     []OrderItem
	Status    OrderStatus
	Version   int
	CreatedAt time.Time
}

func NewOrder(userID uuid.UUID) Order {
	return Order{
		ID:        uuid.New(),
		UserID:    userID,
		Items:     []OrderItem{},
		Status:    OrderStatusPending,
		Version:   1,
		CreatedAt: time.Now(),
	}
}

func (o *Order) AddItem(product Product, quantity int) {
	for i, item := range o.Items {
		if item.ProductID == product.ID {
			o.Items[i].Quantity += quantity
			return
		}
	}

	o.Items = append(o.Items, OrderItem{
		ID:        uuid.New(),
		ProductID: product.ID,
		Name:      product.Name,
		Price:     product.Price,
		Quantity:  quantity,
	})
}

func (o *Order) RemoveItem(productID uuid.UUID) error {
	for i, item := range o.Items {
		if item.ProductID == productID {
			o.Items = append(o.Items[:i], o.Items[i+1:]...)
			return nil
		}
	}
	return ErrItemNotFound
}

func (o *Order) CalculateTotal() float64 {
	total := 0.0
	for _, item := range o.Items {
		total += item.Subtotal()
	}
	return total
}

func (o *Order) Pay() error {
	if o.Status == OrderStatusPaid {
		return ErrOrderAlreadyPaid
	}
	if o.Status == OrderStatusCancelled {
		return ErrOrderAlreadyCancelled
	}
	if len(o.Items) == 0 {
		return ErrEmptyOrder
	}
	o.Status = OrderStatusPaid
	return nil
}

func (o *Order) Cancel() error {
	if o.Status == OrderStatusPaid {
		return ErrOrderAlreadyPaid
	}
	if o.Status == OrderStatusCancelled {
		return ErrOrderAlreadyCancelled
	}
	o.Status = OrderStatusCancelled
	return nil
}
