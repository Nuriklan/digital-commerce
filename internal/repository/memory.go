package repository

import (
	"context"
	"sync"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/google/uuid"
)

// --- Memory Transaction Manager ---

type MemoryTxManager struct {
	mu sync.Mutex
}

func NewMemoryTxManager() *MemoryTxManager {
	return &MemoryTxManager{}
}

func (m *MemoryTxManager) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(ctx)
}

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

func (r *UserMemoryRepository) Save(ctx context.Context, user domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[user.ID] = user
	return nil
}

func (r *UserMemoryRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
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

func (r *ProductMemoryRepository) Save(ctx context.Context, p domain.Product) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.products[p.ID] = p
	return nil
}

func (r *ProductMemoryRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.products[id]
	if !ok {
		return domain.Product{}, ErrNotFound
	}
	return p, nil
}

func (r *ProductMemoryRepository) List(ctx context.Context, limit, offset int) ([]domain.Product, error) {
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

func (r *OrderMemoryRepository) Save(ctx context.Context, o domain.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.orders[o.ID]; ok {
		o.Version = existing.Version + 1
	} else if o.Version == 0 {
		o.Version = 1
	}

	r.orders[o.ID] = o
	return nil
}

func (r *OrderMemoryRepository) SaveOptimistic(ctx context.Context, o domain.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.orders[o.ID]
	if !ok {
		return ErrNotFound
	}

	if existing.Version != o.Version {
		return domain.ErrOptimisticLockConflict
	}

	o.Version = existing.Version + 1
	r.orders[o.ID] = o
	return nil
}

func (r *OrderMemoryRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.orders[id]
	if !ok {
		return domain.Order{}, ErrNotFound
	}
	return o, nil
}

func (r *OrderMemoryRepository) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (domain.Order, error) {
	// In-memory simulation: acquiring a read lock for the order
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

func (r *PaymentMemoryRepository) Save(ctx context.Context, p domain.Payment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payments[p.ID] = p
	return nil
}

func (r *PaymentMemoryRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Payment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.payments[id]
	if !ok {
		return domain.Payment{}, ErrNotFound
	}
	return p, nil
}

// --- Idempotency Memory Repository ---

type IdempotencyMemoryRepository struct {
	mu      sync.RWMutex
	records map[string]domain.IdempotencyRecord
}

func NewIdempotencyMemoryRepository() *IdempotencyMemoryRepository {
	return &IdempotencyMemoryRepository{
		records: make(map[string]domain.IdempotencyRecord),
	}
}

func (r *IdempotencyMemoryRepository) Get(ctx context.Context, key string) (*domain.IdempotencyRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.records[key]
	if !ok {
		return nil, nil
	}
	return &rec, nil
}

func (r *IdempotencyMemoryRepository) Save(ctx context.Context, record domain.IdempotencyRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.records[record.Key]; !ok {
		r.records[record.Key] = record
	}
	return nil
}

// --- Outbox Memory Repository ---
type OutboxMemoryRepository struct {
	mu      sync.RWMutex
	records map[uuid.UUID]domain.OutboxRecord
}

func NewOutboxMemoryRepository() *OutboxMemoryRepository {
	return &OutboxMemoryRepository{
		records: make(map[uuid.UUID]domain.OutboxRecord),
	}
}

func (r *OutboxMemoryRepository) Save(ctx context.Context, record domain.OutboxRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[record.ID] = record
	return nil
}

func (r *OutboxMemoryRepository) FetchPending(ctx context.Context, batchSize int) ([]domain.OutboxRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var pending []domain.OutboxRecord
	for _, rec := range r.records {
		if rec.Status == domain.OutboxStatusPending {
			pending = append(pending, rec)
			if len(pending) >= batchSize {
				break
			}
		}
	}
	return pending, nil
}

func (r *OutboxMemoryRepository) MarkPublished(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.records[id]
	if !ok {
		return ErrNotFound
	}
	now := time.Now().UTC()
	rec.Status = domain.OutboxStatusPublished
	rec.PublishedAt = &now
	r.records[id] = rec
	return nil
}

func (r *OutboxMemoryRepository) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.records[id]
	if !ok {
		return ErrNotFound
	}
	rec.RetryCount++
	rec.ErrorMessage = &errMsg
	if rec.RetryCount >= 5 {
		rec.Status = domain.OutboxStatusFailed
	}
	r.records[id] = rec
	return nil
}

func (r *OutboxMemoryRepository) GetAll() []domain.OutboxRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]domain.OutboxRecord, 0, len(r.records))
	for _, rec := range r.records {
		res = append(res, rec)
	}
	return res
}
