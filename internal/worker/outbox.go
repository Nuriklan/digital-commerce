package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/repository"
)

type EventPublisher interface {
	Publish(ctx context.Context, key string, payload []byte) error
}

type OutboxWorker struct {
	outboxRepo   repository.OutboxRepository
	publisher    EventPublisher
	pollInterval time.Duration
	batchSize    int
	wg           sync.WaitGroup
}

func NewOutboxWorker(
	outboxRepo repository.OutboxRepository,
	publisher EventPublisher,
	pollInterval time.Duration,
	batchSize int,
) *OutboxWorker {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	if batchSize <= 0 {
		batchSize = 20
	}

	return &OutboxWorker{
		outboxRepo:   outboxRepo,
		publisher:    publisher,
		pollInterval: pollInterval,
		batchSize:    batchSize,
	}
}

func (w *OutboxWorker) Start(ctx context.Context) {
	w.wg.Add(1)
	defer w.wg.Done()

	log.Printf("[Outbox Worker] started with interval %v, batchSize %d", w.pollInterval, w.batchSize)

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	w.processBatch(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("[Outbox Worker] stopping gracefully...")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *OutboxWorker) Wait() {
	w.wg.Wait()
}

func (w *OutboxWorker) processBatch(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	records, err := w.outboxRepo.FetchPending(ctx, w.batchSize)
	if err != nil {
		log.Printf("[Outbox Worker] error fetching pending records: %v", err)
		return
	}

	if len(records) == 0 {
		return
	}

	for _, rec := range records {
		if ctx.Err() != nil {
			return
		}

		key := rec.AggregateID.String()
		err := w.publisher.Publish(ctx, key, rec.Payload)
		if err != nil {
			log.Printf("[Outbox Worker] failed to publish record %s (aggregate: %s): %v", rec.ID, rec.AggregateID, err)
			if markErr := w.outboxRepo.MarkFailed(ctx, rec.ID, err.Error()); markErr != nil {
				log.Printf("[Outbox Worker] failed to mark record %s as failed: %v", rec.ID, markErr)
			}
			continue
		}

		if markErr := w.outboxRepo.MarkPublished(ctx, rec.ID); markErr != nil {
			log.Printf("[Outbox Worker] failed to mark record %s as published: %v", rec.ID, markErr)
		} else {
			log.Printf("[Outbox Worker] successfully published event for order %s to topic %s", rec.AggregateID, rec.Topic)
		}
	}
}
