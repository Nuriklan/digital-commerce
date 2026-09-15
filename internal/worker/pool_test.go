package worker_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/worker"
)

func TestPool_ProcessesAllJobs(t *testing.T) {
	var processedCount int64
	totalJobs := 50

	handler := func(ctx context.Context, job worker.Job) error {
		atomic.AddInt64(&processedCount, 1)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := worker.NewPool(4, 10, handler)
	pool.Start(ctx)

	for i := 0; i < totalJobs; i++ {
		submitted := pool.Submit(ctx, worker.Job{
			Name:    "test_job",
			Payload: i,
		})
		if !submitted {
			t.Fatalf("failed to submit job %d", i)
		}
	}

	pool.Wait()

	if count := atomic.LoadInt64(&processedCount); count != int64(totalJobs) {
		t.Errorf("expected %d processed jobs, got %d", totalJobs, count)
	}
}

func TestPool_ContextCancellation(t *testing.T) {
	handler := func(ctx context.Context, job worker.Job) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	pool := worker.NewPool(2, 5, handler)
	pool.Start(ctx)

	// Waiting for the context timeout to expire
	<-ctx.Done()

	// An attempt to send after cancellation should return false
	submitted := pool.Submit(ctx, worker.Job{Name: "late_job"})
	if submitted {
		t.Errorf("expected submit to return false after context cancellation")
	}

	pool.Wait()
}
