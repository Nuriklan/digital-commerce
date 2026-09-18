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

func NewPostgresDB(cfg config.DatabaseConfig) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return db, nil
}

// --- User Postgres Repository ---
type UserPostgresRepository struct {
	db *sql.DB
}

func NewUserPostgresRepository(db *sql.DB) *UserPostgresRepository {
	return &UserPostgresRepository{db: db}
}

func (r *UserPostgresRepository) Save(user domain.User) error {
	query := `
		INSERT INTO users (id, name, email, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name, email = EXCLUDED.email;
	`

	_, err := r.db.Exec(query, user.ID, user.Name, user.Email, user.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}
	return nil
}

func (r *UserPostgresRepository) GetByID(id uuid.UUID) (domain.User, error) {
	query := `SELECT id, name, email, created_at FROM users WHERE id = $1`
	var u domain.User

	err := r.db.QueryRow(query, id).Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrNotFound
		}
		return domain.User{}, fmt.Errorf("failed to get user: %w", err)
	}
	return u, nil
}

// --- Product Postgres Repository ---

type ProductPostgresRepository struct {
	db *sql.DB
}

func NewProductPostgresRepository(db *sql.DB) *ProductPostgresRepository {
	return &ProductPostgresRepository{db: db}
}

func (r *ProductPostgresRepository) Save(p domain.Product) error {
	query := `
		INSERT INTO products (id, name, price)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name, price = EXCLUDED.price;
	`
	_, err := r.db.Exec(query, p.ID, p.Name, p.Price)
	if err != nil {
		return fmt.Errorf("failed to save product: %w", err)
	}
	return nil
}

func (r *ProductPostgresRepository) GetByID(id uuid.UUID) (domain.Product, error) {
	query := `SELECT id, name, price FROM products WHERE id = $1`
	var p domain.Product

	err := r.db.QueryRow(query, id).Scan(&p.ID, &p.Name, &p.Price)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Product{}, ErrNotFound
		}
		return domain.Product{}, fmt.Errorf("failed to get product: %w", err)
	}
	return p, nil
}

func (r *ProductPostgresRepository) List(limit, offset int) ([]domain.Product, error) {
	query := `SELECT id, name, price FROM products ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := r.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list products: %w", err)
	}
	defer rows.Close()

	var products []domain.Product
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Price); err != nil {
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

func (r *OrderPostgresRepository) Save(o domain.Order) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	orderQuery := `
		INSERT INTO orders (id, user_id, status, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;
	`
	if _, err := tx.Exec(orderQuery, o.ID, o.UserID, string(o.Status), o.CreatedAt); err != nil {
		return fmt.Errorf("failed to upsert order: %w", err)
	}

	itemQuery := `
		INSERT INTO order_items (id, order_id, product_id, price, quantity)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING;
	`
	for _, item := range o.Items {
		if _, err := tx.Exec(itemQuery, item.ID, o.ID, item.ProductID, item.Price, item.Quantity); err != nil {
			return fmt.Errorf("failed to insert order item: %w", err)
		}
	}

	return tx.Commit()
}

func (r *OrderPostgresRepository) GetByID(id uuid.UUID) (domain.Order, error) {
	orderQuery := `SELECT id, user_id, status, created_at FROM orders WHERE id = $1`
	var o domain.Order
	var statusStr string

	err := r.db.QueryRow(orderQuery, id).Scan(&o.ID, &o.UserID, &statusStr, &o.CreatedAt)
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
	rows, err := r.db.Query(itemsQuery, id)
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

// --- Payment Postgres Repository ---

type PaymentPostgresRepository struct {
	db *sql.DB
}

func NewPaymentPostgresRepository(db *sql.DB) *PaymentPostgresRepository {
	return &PaymentPostgresRepository{db: db}
}

func (r *PaymentPostgresRepository) Save(p domain.Payment) error {
	query := `
		INSERT INTO payments (id, order_id, amount, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;
	`
	_, err := r.db.Exec(query, p.ID, p.OrderID, p.Amount, string(p.Status), p.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to save payment: %w", err)
	}
	return nil
}

func (r *PaymentPostgresRepository) GetByID(id uuid.UUID) (domain.Payment, error) {
	query := `SELECT id, order_id, amount, status, created_at FROM payments WHERE id = $1`
	var p domain.Payment
	var statusStr string

	err := r.db.QueryRow(query, id).Scan(&p.ID, &p.OrderID, &p.Amount, &statusStr, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Payment{}, ErrNotFound
		}
		return domain.Payment{}, fmt.Errorf("failed to get payment: %w", err)
	}
	p.Status = domain.PaymentStatus(statusStr)
	return p, nil
}
