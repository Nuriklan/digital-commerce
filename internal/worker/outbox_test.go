package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/worker"
	"github.com/google/uuid"
)

type mockPublisher struct {
	mu        sync.Mutex
	published map[string][]byte
	shouldErr bool
}

func newMockPublisher(shouldErr bool) *mockPublisher {
	return &mockPublisher{
		published: make(map[string][]byte),
		shouldErr: shouldErr,
	}
}

func (m *mockPublisher) Publish(ctx context.Context, key string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldErr {
		return errors.New("kafka connection failure")
	}

	m.published[key] = payload
	return nil
}

func TestOutboxWorker_Success(t *testing.T) {
	outboxRepo := repository.NewOutboxMemoryRepository()
	pub := newMockPublisher(false)

	orderID := uuid.New()
	rec := domain.NewOutboxRecord("order", orderID, "order.events", []byte(`{"order_id":"test"}`))
	_ = outboxRepo.Save(context.Background(), rec)

	w := worker.NewOutboxWorker(outboxRepo, pub, 50*time.Millisecond, 10)

	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)

	time.Sleep(100 * time.Millisecond)
	cancel()
	w.Wait()

	pub.mu.Lock()
	defer pub.mu.Unlock()

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(pub.published))
	}

	records, _ := outboxRepo.FetchPending(context.Background(), 10)
	if len(records) != 0 {
		t.Fatalf("expected 0 pending records left, got %d", len(records))
	}
}

func TestOutboxWorker_PublishError_Retries(t *testing.T) {
	outboxRepo := repository.NewOutboxMemoryRepository()
	pub := newMockPublisher(true)

	orderID := uuid.New()
	rec := domain.NewOutboxRecord("order", orderID, "order.events", []byte(`{"order_id":"fail"}`))
	_ = outboxRepo.Save(context.Background(), rec)

	w := worker.NewOutboxWorker(outboxRepo, pub, 50*time.Millisecond, 10)

	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)

	time.Sleep(100 * time.Millisecond)
	cancel()
	w.Wait()

	records, _ := outboxRepo.FetchPending(context.Background(), 10)
	if len(records) != 1 {
		t.Fatalf("expected record to stay pending for retry, got %d", len(records))
	}

	if records[0].RetryCount == 0 {
		t.Errorf("expected RetryCount > 0, got %d", records[0].RetryCount)
	}
}
