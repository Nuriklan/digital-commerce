package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/config"
	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type txKey struct{}

func getExecutor(ctx context.Context, defaultDB *sql.DB) DBExecutor {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok && tx != nil {
		return tx
	}
	return defaultDB
}

func NewPostgresDB(cfg config.DatabaseConfig) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return db, nil
}

// --- Postgres Transaction Manager ---

type PostgresTxManager struct {
	db *sql.DB
}

func NewPostgresTxManager(db *sql.DB) *PostgresTxManager {
	return &PostgresTxManager{db: db}
}

func (m *PostgresTxManager) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	// If already in a transaction, reuse it
	if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return fn(ctx)
	}

	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	txCtx := context.WithValue(ctx, txKey{}, tx)

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(txCtx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("transaction error: %w, rollback error: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// --- User Postgres Repository ---

type UserPostgresRepository struct {
	db *sql.DB
}

func NewUserPostgresRepository(db *sql.DB) *UserPostgresRepository {
	return &UserPostgresRepository{db: db}
}

func (r *UserPostgresRepository) Save(ctx context.Context, user domain.User) error {
	exec := getExecutor(ctx, r.db)
	query := `
		INSERT INTO users (id, name, email, password_hash, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
			email = EXCLUDED.email,
			password_hash = EXCLUDED.password_hash,
			role = EXCLUDED.role;
	`

	_, err := exec.ExecContext(ctx, query, user.ID, user.Name, user.Email, user.PasswordHash, string(user.Role), user.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}
	return nil
}

func (r *UserPostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	exec := getExecutor(ctx, r.db)
	query := `SELECT id, name, email, password_hash, role, created_at FROM users WHERE id = $1`
	var u domain.User
	var roleStr string

	err := exec.QueryRowContext(ctx, query, id).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &roleStr, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrNotFound
		}
		return domain.User{}, fmt.Errorf("failed to get user: %w", err)
	}
	u.Role = domain.Role(roleStr)
	return u, nil
}

func (r *UserPostgresRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	exec := getExecutor(ctx, r.db)
	query := `SELECT id, name, email, password_hash, role, created_at FROM users WHERE email = $1`
	var u domain.User
	var roleStr string

	err := exec.QueryRowContext(ctx, query, email).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &roleStr, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrNotFound
		}
		return domain.User{}, fmt.Errorf("failed to get user by email: %w", err)
	}
	u.Role = domain.Role(roleStr)
	return u, nil
}

// --- Product Postgres Repository ---

type ProductPostgresRepository struct {
	db *sql.DB
}

func NewProductPostgresRepository(db *sql.DB) *ProductPostgresRepository {
	return &ProductPostgresRepository{db: db}
}

func (r *ProductPostgresRepository) Save(ctx context.Context, p domain.Product) error {
	exec := getExecutor(ctx, r.db)
	query := `
		INSERT INTO products (id, name, price, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name, price = EXCLUDED.price;
	`
	_, err := exec.ExecContext(ctx, query, p.ID, p.Name, p.Price, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to save product: %w", err)
	}
	return nil
}

func (r *ProductPostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	exec := getExecutor(ctx, r.db)
	query := `SELECT id, name, price, created_at FROM products WHERE id = $1`
	var p domain.Product

	err := exec.QueryRowContext(ctx, query, id).Scan(&p.ID, &p.Name, &p.Price, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Product{}, ErrNotFound
		}
		return domain.Product{}, fmt.Errorf("failed to get product: %w", err)
	}
	return p, nil
}

func (r *ProductPostgresRepository) List(ctx context.Context, limit, offset int) ([]domain.Product, error) {
	exec := getExecutor(ctx, r.db)
	query := `SELECT id, name, price, created_at FROM products ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := exec.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list products: %w", err)
	}
	defer rows.Close()

	var products []domain.Product
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Price, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	if products == nil {
		products = []domain.Product{}
	}
	return products, nil
}

// --- Order Postgres Repository ---

type OrderPostgresRepository struct {
	db *sql.DB
}

func NewOrderPostgresRepository(db *sql.DB) *OrderPostgresRepository {
	return &OrderPostgresRepository{db: db}
}

func (r *OrderPostgresRepository) saveWithExecutor(ctx context.Context, exec DBExecutor, o domain.Order) error {
	orderQuery := `
		INSERT INTO orders (id, user_id, status, version, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET 
			status = EXCLUDED.status,
			version = orders.version + 1;
	`
	if _, err := exec.ExecContext(ctx, orderQuery, o.ID, o.UserID, string(o.Status), o.Version, o.CreatedAt); err != nil {
		return fmt.Errorf("failed to upsert order: %w", err)
	}

	itemQuery := `
		INSERT INTO order_items (id, order_id, product_id, price, quantity)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING;
	`
	for _, item := range o.Items {
		if _, err := exec.ExecContext(ctx, itemQuery, item.ID, o.ID, item.ProductID, item.Price, item.Quantity); err != nil {
			return fmt.Errorf("failed to insert order item: %w", err)
		}
	}
	return nil
}

func (r *OrderPostgresRepository) Save(ctx context.Context, o domain.Order) error {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok && tx != nil {
		return r.saveWithExecutor(ctx, tx, o)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := r.saveWithExecutor(ctx, tx, o); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *OrderPostgresRepository) SaveOptimistic(ctx context.Context, o domain.Order) error {
	exec := getExecutor(ctx, r.db)
	query := `
		UPDATE orders 
		SET status = $1, version = version + 1 
		WHERE id = $2 AND version = $3
	`
	res, err := exec.ExecContext(ctx, query, string(o.Status), o.ID, o.Version)
	if err != nil {
		return fmt.Errorf("failed to update order optimistically: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		var exists bool
		checkErr := exec.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM orders WHERE id = $1)", o.ID).Scan(&exists)
		if checkErr == nil && exists {
			return domain.ErrOptimisticLockConflict
		}
		return ErrNotFound
	}

	return nil
}

func (r *OrderPostgresRepository) getOrderByQuery(ctx context.Context, query string, id uuid.UUID) (domain.Order, error) {
	exec := getExecutor(ctx, r.db)
	var o domain.Order
	var statusStr string

	err := exec.QueryRowContext(ctx, query, id).Scan(&o.ID, &o.UserID, &statusStr, &o.Version, &o.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Order{}, ErrNotFound
		}
		return domain.Order{}, fmt.Errorf("failed to get order: %w", err)
	}
	o.Status = domain.OrderStatus(statusStr)

	itemsQuery := `
		SELECT oi.id, oi.product_id, p.name, oi.price, oi.quantity
		FROM order_items oi
		JOIN products p ON p.id = oi.product_id
		WHERE oi.order_id = $1
	`
	rows, err := exec.QueryContext(ctx, itemsQuery, id)
	if err != nil {
		return domain.Order{}, fmt.Errorf("failed to get order items: %w", err)
	}
	defer rows.Close()

	o.Items = []domain.OrderItem{}
	for rows.Next() {
		var item domain.OrderItem
		if err := rows.Scan(&item.ID, &item.ProductID, &item.Name, &item.Price, &item.Quantity); err != nil {
			return domain.Order{}, fmt.Errorf("failed to scan order item: %w", err)
		}
		o.Items = append(o.Items, item)
	}

	return o, nil
}

func (r *OrderPostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Order, error) {
	query := `SELECT id, user_id, status, version, created_at FROM orders WHERE id = $1`
	return r.getOrderByQuery(ctx, query, id)
}

func (r *OrderPostgresRepository) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (domain.Order, error) {
	query := `SELECT id, user_id, status, version, created_at FROM orders WHERE id = $1 FOR UPDATE`
	return r.getOrderByQuery(ctx, query, id)
}

// --- Payment Postgres Repository ---

type PaymentPostgresRepository struct {
	db *sql.DB
}

func NewPaymentPostgresRepository(db *sql.DB) *PaymentPostgresRepository {
	return &PaymentPostgresRepository{db: db}
}

func (r *PaymentPostgresRepository) Save(ctx context.Context, p domain.Payment) error {
	exec := getExecutor(ctx, r.db)
	query := `
		INSERT INTO payments (id, order_id, amount, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;
	`
	_, err := exec.ExecContext(ctx, query, p.ID, p.OrderID, p.Amount, string(p.Status), p.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to save payment: %w", err)
	}
	return nil
}

func (r *PaymentPostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Payment, error) {
	exec := getExecutor(ctx, r.db)
	query := `SELECT id, order_id, amount, status, created_at FROM payments WHERE id = $1`
	var p domain.Payment
	var statusStr string

	err := exec.QueryRowContext(ctx, query, id).Scan(&p.ID, &p.OrderID, &p.Amount, &statusStr, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Payment{}, ErrNotFound
		}
		return domain.Payment{}, fmt.Errorf("failed to get payment: %w", err)
	}
	p.Status = domain.PaymentStatus(statusStr)
	return p, nil
}

// --- Idempotency Postgres Repository ---

type IdempotencyPostgresRepository struct {
	db *sql.DB
}

func NewIdempotencyPostgresRepository(db *sql.DB) *IdempotencyPostgresRepository {
	return &IdempotencyPostgresRepository{db: db}
}

func (r *IdempotencyPostgresRepository) Get(ctx context.Context, key string) (*domain.IdempotencyRecord, error) {
	exec := getExecutor(ctx, r.db)
	query := `SELECT key, payment_id, order_id, status_code, response_body, created_at FROM idempotency_keys WHERE key = $1`
	var rec domain.IdempotencyRecord

	err := exec.QueryRowContext(ctx, query, key).Scan(
		&rec.Key, &rec.PaymentID, &rec.OrderID, &rec.StatusCode, &rec.ResponseBody, &rec.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get idempotency key: %w", err)
	}
	return &rec, nil
}

func (r *IdempotencyPostgresRepository) Save(ctx context.Context, record domain.IdempotencyRecord) error {
	exec := getExecutor(ctx, r.db)
	query := `
		INSERT INTO idempotency_keys (key, payment_id, order_id, status_code, response_body, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (key) DO NOTHING;
	`
	_, err := exec.ExecContext(ctx, query, record.Key, record.PaymentID, record.OrderID, record.StatusCode, record.ResponseBody, record.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to save idempotency record: %w", err)
	}
	return nil
}
