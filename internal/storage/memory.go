package storage

import (
	"errors"
	"sync"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("entity not found")
)

type MemoryStorage struct {
	muUsers    sync.RWMutex
	users      map[uuid.UUID]domain.User
	muProducts sync.RWMutex
	products   map[uuid.UUID]domain.Product
	muOrders   sync.RWMutex
	orders     map[uuid.UUID]domain.Order
	muPayments sync.RWMutex
	payments   map[uuid.UUID]domain.Payment
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		users:    make(map[uuid.UUID]domain.User),
		products: make(map[uuid.UUID]domain.Product),
		orders:   make(map[uuid.UUID]domain.Order),
		payments: make(map[uuid.UUID]domain.Payment),
	}
}

// --- Users ---

func (s *MemoryStorage) SaveUser(user domain.User) {
	s.muUsers.Lock()
	defer s.muUsers.Unlock()
	s.users[user.ID] = user
}

func (s *MemoryStorage) GetUserByID(id uuid.UUID) (domain.User, error) {
	s.muUsers.RLock()
	defer s.muUsers.RUnlock()

	user, ok := s.users[id]
	if !ok {
		return domain.User{}, ErrNotFound
	}
	return user, nil
}

// --- Products ---

func (s *MemoryStorage) SaveProduct(p domain.Product) {
	s.muProducts.Lock()
	defer s.muProducts.Unlock()
	s.products[p.ID] = p
}

func (s *MemoryStorage) GetProductByID(id uuid.UUID) (domain.Product, error) {
	s.muProducts.RLock()
	defer s.muProducts.RUnlock()

	p, ok := s.products[id]
	if !ok {
		return domain.Product{}, ErrNotFound
	}
	return p, nil
}

func (s *MemoryStorage) ListProducts(limit, offset int) []domain.Product {
	s.muProducts.RLock()
	defer s.muProducts.RUnlock()

	all := make([]domain.Product, 0, len(s.products))
	for _, p := range s.products {
		all = append(all, p)
	}

	if offset >= len(all) {
		return []domain.Product{}
	}

	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end]
}

// --- Orders ---

func (s *MemoryStorage) SaveOrder(o domain.Order) {
	s.muOrders.Lock()
	defer s.muOrders.Unlock()
	s.orders[o.ID] = o
}

func (s *MemoryStorage) GetOrderByID(id uuid.UUID) (domain.Order,
	error) {
	s.muOrders.RLock()
	defer s.muOrders.RUnlock()

	o, ok := s.orders[id]
	if !ok {
		return domain.Order{}, ErrNotFound
	}
	return o, nil
}

// --- Payments ---

func (s *MemoryStorage) SavePayment(p domain.Payment) {
	s.muPayments.Lock()
	defer s.muPayments.Unlock()
	s.payments[p.ID] = p
}

func (s *MemoryStorage) GetPaymentByID(id uuid.UUID) (domain.Payment,
	error) {
	s.muPayments.RLock()
	defer s.muPayments.RUnlock()

	p, ok := s.payments[id]
	if !ok {
		return domain.Payment{}, ErrNotFound
	}
	return p, nil
}
