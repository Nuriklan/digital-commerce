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
