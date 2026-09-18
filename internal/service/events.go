package service

import (
	"context"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/worker"
)

type OrderEventPublisher interface {
	PublishOrderCreated(ctx context.Context, order domain.Order) error
}

type WorkerPoolEventPublisher struct {
	pool *worker.Pool
}

func NewWorkerPoolEventPublisher(pool *worker.Pool) *WorkerPoolEventPublisher {
	return &WorkerPoolEventPublisher{pool: pool}
}

func (p *WorkerPoolEventPublisher) PublishOrderCreated(ctx context.Context, order domain.Order) error {
	if p.pool == nil {
		return nil
	}
	p.pool.Submit(ctx, worker.Job{
		Name:    "send_notification",
		Payload: order,
	})
	p.pool.Submit(ctx, worker.Job{
		Name:    "update_analytics",
		Payload: order,
	})
	return nil
}
