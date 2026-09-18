package repository

import (
	"sync"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/google/uuid"
)

// --- User Memory Repository ---

type UserMemoryRepository struct {
	mu    sync.RWMutex
	users map[uuid.UUID]domain.User
}

func NewUserMemoryRepository() *UserMemoryRepository {
	return &UserMemoryRepository{
		users: make(map[uuid.UUID]domain.User),
	}
}

func (r *UserMemoryRepository) Save(user domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[user.ID] = user
	return nil
}

func (r *UserMemoryRepository) GetByID(id uuid.UUID) (domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	user, ok := r.users[id]
	if !ok {
		return domain.User{}, ErrNotFound
	}
	return user, nil
}

// --- Product Memory Repository ---

type ProductMemoryRepository struct {
	mu       sync.RWMutex
	products map[uuid.UUID]domain.Product
}

func NewProductMemoryRepository() *ProductMemoryRepository {
	return &ProductMemoryRepository{
		products: make(map[uuid.UUID]domain.Product),
	}
}

func (r *ProductMemoryRepository) Save(p domain.Product) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.products[p.ID] = p
	return nil
}

func (r *ProductMemoryRepository) GetByID(id uuid.UUID) (domain.Product, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.products[id]
	if !ok {
		return domain.Product{}, ErrNotFound
	}
	return p, nil
}

func (r *ProductMemoryRepository) List(limit, offset int) ([]domain.Product, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]domain.Product, 0, len(r.products))
	for _, p := range r.products {
		all = append(all, p)
	}

	if offset >= len(all) {
		return []domain.Product{}, nil
	}

	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end], nil
}

// --- Order Memory Repository ---

type OrderMemoryRepository struct {
	mu     sync.RWMutex
	orders map[uuid.UUID]domain.Order
}

func NewOrderMemoryRepository() *OrderMemoryRepository {
	return &OrderMemoryRepository{
		orders: make(map[uuid.UUID]domain.Order),
	}
}

func (r *OrderMemoryRepository) Save(o domain.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orders[o.ID] = o
	return nil
}

func (r *OrderMemoryRepository) GetByID(id uuid.UUID) (domain.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.orders[id]
	if !ok {
		return domain.Order{}, ErrNotFound
	}
	return o, nil
}

// --- Payment Memory Repository ---

type PaymentMemoryRepository struct {
	mu       sync.RWMutex
	payments map[uuid.UUID]domain.Payment
}

func NewPaymentMemoryRepository() *PaymentMemoryRepository {
	return &PaymentMemoryRepository{
		payments: make(map[uuid.UUID]domain.Payment),
	}
}

func (r *PaymentMemoryRepository) Save(p domain.Payment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payments[p.ID] = p
	return nil
}

func (r *PaymentMemoryRepository) GetByID(id uuid.UUID) (domain.Payment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.payments[id]
	if !ok {
		return domain.Payment{}, ErrNotFound
	}
	return p, nil
}
