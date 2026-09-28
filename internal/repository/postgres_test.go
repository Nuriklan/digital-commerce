package repository_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/config"
	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func setupTestDB(t *testing.T) (*sql.DB, *repository.PostgresTxManager) {
	t.Helper()
	cfg := config.DatabaseConfig{
		DSN:             "postgres://commerce_user:commerce_password@localhost:5432/digital_commerce?sslmode=disable",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}

	db, err := repository.NewPostgresDB(cfg)
	if err != nil {
		t.Skipf("Skipping Postgres integration test (database not reachable): %v", err)
	}

	txManager := repository.NewPostgresTxManager(db)
	return db, txManager
}

func TestPostgresTxManager_RollbackOnError(t *testing.T) {
	db, txManager := setupTestDB(t)
	defer db.Close()

	userRepo := repository.NewUserPostgresRepository(db)
	ctx := context.Background()

	testUser, _ := domain.NewUser("Rollback User", fmt.Sprintf("rollback-%s@example.com", uuid.New()))

	// Run within transaction and simulate failure
	errIntentional := errors.New("something went wrong after insert")
	err := txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := userRepo.Save(txCtx, testUser); err != nil {
			return err
		}
		// Failure before commit -> should rollback
		return errIntentional
	})

	if !errors.Is(err, errIntentional) {
		t.Fatalf("expected intentional error, got %v", err)
	}

	// Verify user was NOT saved (rolled back)
	_, err = userRepo.GetByID(ctx, testUser.ID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound because transaction was rolled back, got %v", err)
	}
}

func TestPostgres_PessimisticLocking_RowLock(t *testing.T) {
	db, txManager := setupTestDB(t)
	defer db.Close()

	userRepo := repository.NewUserPostgresRepository(db)
	productRepo := repository.NewProductPostgresRepository(db)
	orderRepo := repository.NewOrderPostgresRepository(db)
	ctx := context.Background()

	// Seed user & product & order
	user, _ := domain.NewUser("Lock User", fmt.Sprintf("lock-%s@example.com", uuid.New()))
	_ = userRepo.Save(ctx, user)

	prod, _ := domain.NewProduct("Lock Product", 10.0)
	_ = productRepo.Save(ctx, prod)

	order := domain.NewOrder(user.ID)
	order.AddItem(prod, 1)
	if err := orderRepo.Save(ctx, order); err != nil {
		t.Fatalf("failed to seed order: %v", err)
	}

	// Concurrency test: Tx 1 locks order for 200ms and updates status to paid.
	// Tx 2 simultaneously tries to GetByIDForUpdate. It must block until Tx 1 commits,
	// and observe status = paid!
	tx1Started := make(chan struct{})
	tx1Committed := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Tx 1 holds lock
	go func() {
		defer wg.Done()
		err := txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
			o, err := orderRepo.GetByIDForUpdate(txCtx, order.ID)
			if err != nil {
				return err
			}
			close(tx1Started)

			time.Sleep(200 * time.Millisecond) // hold the lock
			_ = o.Pay()
			return orderRepo.Save(txCtx, o)
		})
		if err != nil {
			t.Errorf("Tx1 failed: %v", err)
		}
		close(tx1Committed)
	}()

	// Goroutine 2: Tx 2 waits for lock
	var tx2ObservedStatus domain.OrderStatus
	go func() {
		defer wg.Done()
		<-tx1Started // wait until Tx 1 acquires the lock

		start := time.Now()
		err := txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
			// This call must block until Tx 1 commits!
			o, err := orderRepo.GetByIDForUpdate(txCtx, order.ID)
			if err != nil {
				return err
			}
			tx2ObservedStatus = o.Status
			return nil
		})

		elapsed := time.Since(start)
		if elapsed < 150*time.Millisecond {
			t.Errorf("Tx2 did not block on row lock! Elapsed: %v", elapsed)
		}

		if err != nil {
			t.Errorf("Tx2 failed: %v", err)
		}
	}()

	wg.Wait()

	if tx2ObservedStatus != domain.OrderStatusPaid {
		t.Errorf("expected Tx2 to observe committed status 'paid', got '%s'", tx2ObservedStatus)
	}
}

func TestPostgres_IdempotencyRepository(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	repo := repository.NewIdempotencyPostgresRepository(db)
	ctx := context.Background()

	key := fmt.Sprintf("idemp-pg-test-%s", uuid.New())
	record := domain.IdempotencyRecord{
		Key:          key,
		PaymentID:    uuid.New(),
		OrderID:      uuid.New(),
		StatusCode:   201,
		ResponseBody: []byte(`{"status":"success"}`),
		CreatedAt:    time.Now().UTC().Truncate(time.Microsecond),
	}

	if err := repo.Save(ctx, record); err != nil {
		t.Fatalf("failed to save idempotency record: %v", err)
	}

	fetched, err := repo.Get(ctx, key)
	if err != nil {
		t.Fatalf("failed to get idempotency record: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected record to be found")
	}

	if fetched.Key != key {
		t.Errorf("expected key %s, got %s", key, fetched.Key)
	}
	if fetched.PaymentID != record.PaymentID {
		t.Errorf("expected payment ID %s, got %s", record.PaymentID, fetched.PaymentID)
	}
	var expectedMap, actualMap map[string]any
	_ = json.Unmarshal(record.ResponseBody, &expectedMap)
	_ = json.Unmarshal(fetched.ResponseBody, &actualMap)
	if expectedMap["status"] != actualMap["status"] {
		t.Errorf("expected status %v, got %v", expectedMap["status"], actualMap["status"])
	}
}

func TestPostgres_OutboxRepository_Lifecycle(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	repo := repository.NewPostgresOutboxRepository(db)
	ctx := context.Background()

	// 1. Creating test Outbox-record
	orderID := uuid.New()
	payload := []byte(fmt.Sprintf(`{"order_id":"%s","total":150.00}`, orderID))
	record := domain.NewOutboxRecord("order", orderID, "order.events", payload)

	// Guaranteed cleanup of the record after test completion
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM outbox WHERE id = $1", record.ID)
	})

	// 2. Saving record in PostgreSQL
	if err := repo.Save(ctx, record); err != nil {
		t.Fatalf("failed to save outbox record: %v", err)
	}

	// 3. Select PENDING records by FetchPending
	pending, err := repo.FetchPending(ctx, 10)
	if err != nil {
		t.Fatalf("failed to fetch pending outbox records: %v", err)
	}

	var foundRecord *domain.OutboxRecord
	for _, r := range pending {
		if r.ID == record.ID {
			recCopy := r
			foundRecord = &recCopy
			break
		}
	}

	if foundRecord == nil {
		t.Fatalf("saved outbox record %s not found in pending list", record.ID)
	}
	if foundRecord.Status != domain.OutboxStatusPending {
		t.Errorf("expected status %s, got %s", domain.OutboxStatusPending, foundRecord.Status)
	}
	var expectedPayload, actualPayload map[string]any
	if err := json.Unmarshal(payload, &expectedPayload); err != nil {
		t.Fatalf("failed to unmarshal expected payload: %v", err)
	}
	if err := json.Unmarshal(foundRecord.Payload, &actualPayload); err != nil {
		t.Fatalf("failed to unmarshal actual payload: %v", err)
	}
	if expectedPayload["order_id"] != actualPayload["order_id"] {
		t.Errorf("order_id mismatch: expected %v, got %v", expectedPayload["order_id"], actualPayload["order_id"])
	}

	// 4. Getting published record (MarkPublished)
	if err := repo.MarkPublished(ctx, record.ID); err != nil {
		t.Fatalf("failed to mark outbox record as published: %v", err)
	}

	// 5. Checking record not returning in FetchPending
	pendingAfterPublish, err := repo.FetchPending(ctx, 10)
	if err != nil {
		t.Fatalf("failed to fetch pending records after publish: %v", err)
	}

	for _, r := range pendingAfterPublish {
		if r.ID == record.ID {
			t.Errorf("published record %s is still returned in pending list!", record.ID)
		}
	}

	// 6. Verifying MarkFailed behavior and escalation to FAILED status upon exhaustion of retry_count.
	failedRecord := domain.NewOutboxRecord("order", uuid.New(), "order.events", payload)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM outbox WHERE id = $1", failedRecord.ID)
	})

	if err := repo.Save(ctx, failedRecord); err != nil {
		t.Fatalf("failed to save second outbox record: %v", err)
	}

	// Simulating 5 failed publication attempts
	for attempt := 1; attempt <= 5; attempt++ {
		errMsg := fmt.Sprintf("kafka broker unavailable (attempt %d)", attempt)
		if err := repo.MarkFailed(ctx, failedRecord.ID, errMsg); err != nil {
			t.Fatalf("attempt %d failed to MarkFailed: %v", attempt, err)
		}
	}

	// After 5 attempts, the status in the database should become FAILED, and retry_count should be 5
	var status string
	var retryCount int
	var lastErr string
	err = db.QueryRowContext(
		ctx,
		"SELECT status, retry_count, error_message FROM outbox WHERE id = $1",
		failedRecord.ID,
	).Scan(&status, &retryCount, &lastErr)
	if err != nil {
		t.Fatalf("failed to query outbox record status: %v", err)
	}

	if status != "FAILED" {
		t.Errorf("expected status 'FAILED' after 5 retries, got '%s'", status)
	}
	if retryCount != 5 {
		t.Errorf("expected retry_count = 5, got %d", retryCount)
	}
}
