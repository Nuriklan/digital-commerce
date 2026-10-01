package repository

import (
	"context"
	"errors"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("entity not found")
)

// TxManager defines the interface for transaction boundary management.
type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type UserRepository interface {
	Save(ctx context.Context, user domain.User) error
	GetByID(ctx context.Context, id uuid.UUID) (domain.User, error)
	GetByEmail(ctx context.Context, email string) (domain.User, error)
}

type ProductRepository interface {
	Save(ctx context.Context, product domain.Product) error
	GetByID(ctx context.Context, id uuid.UUID) (domain.Product, error)
	List(ctx context.Context, limit, offset int) ([]domain.Product, error)
}

type OrderRepository interface {
	Save(ctx context.Context, order domain.Order) error
	SaveOptimistic(ctx context.Context, order domain.Order) error
	GetByID(ctx context.Context, id uuid.UUID) (domain.Order, error)
	GetByIDForUpdate(ctx context.Context, id uuid.UUID) (domain.Order, error)
}

type PaymentRepository interface {
	Save(ctx context.Context, payment domain.Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (domain.Payment, error)
}

type IdempotencyRepository interface {
	Get(ctx context.Context, key string) (*domain.IdempotencyRecord, error)
	Save(ctx context.Context, record domain.IdempotencyRecord) error
}

type OutboxRepository interface {
	Save(ctx context.Context, record domain.OutboxRecord) error
	FetchPending(ctx context.Context, batchSize int) ([]domain.OutboxRecord, error)
	MarkPublished(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error
}
