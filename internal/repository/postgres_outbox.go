package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/google/uuid"
)

type PostgresOutboxRepository struct {
	db *sql.DB
}

func NewPostgresOutboxRepository(db *sql.DB) *PostgresOutboxRepository {
	return &PostgresOutboxRepository{db: db}
}

func (r *PostgresOutboxRepository) Save(ctx context.Context, record domain.OutboxRecord) error {
	executor := getExecutor(ctx, r.db)

	query := `
		INSERT INTO outbox (
			id, aggregate_type, aggregate_id, topic, payload,
			status, retry_count, error_message, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := executor.ExecContext(
		ctx,
		query,
		record.ID,
		record.AggregateType,
		record.AggregateID,
		record.Topic,
		record.Payload,
		record.Status,
		record.RetryCount,
		record.ErrorMessage,
		record.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save outbox record: %w", err)
	}

	return nil
}

func (r *PostgresOutboxRepository) FetchPending(ctx context.Context, batchSize int) ([]domain.OutboxRecord, error) {
	executor := getExecutor(ctx, r.db)

	query := `
		SELECT id, aggregate_type, aggregate_id, topic, payload,
				status, retry_count, error_message, created_at, published_at
		FROM outbox
		WHERE status = 'PENDING'
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := executor.QueryContext(ctx, query, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pending outbox records: %w", err)
	}
	defer rows.Close()

	var records []domain.OutboxRecord
	for rows.Next() {
		var rec domain.OutboxRecord
		var errStr sql.NullString
		var pubAt sql.NullTime

		err := rows.Scan(
			&rec.ID,
			&rec.AggregateType,
			&rec.AggregateID,
			&rec.Topic,
			&rec.Payload,
			&rec.Status,
			&rec.RetryCount,
			&errStr,
			&rec.CreatedAt,
			&pubAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan outbox record: %w", err)
		}

		if errStr.Valid {
			rec.ErrorMessage = &errStr.String
		}
		if pubAt.Valid {
			rec.PublishedAt = &pubAt.Time
		}

		records = append(records, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error while fetching outbox records: %w", err)
	}

	return records, nil
}

func (r *PostgresOutboxRepository) MarkPublished(ctx context.Context, id uuid.UUID) error {
	executor := getExecutor(ctx, r.db)

	query := `
		UPDATE outbox
		SET status = $1, published_at = $2, error_message = NULL
		WHERE id = $3
	`

	_, err := executor.ExecContext(ctx, query, domain.OutboxStatusPublished, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to mark outbox record as published: %w", err)
	}

	return nil
}

func (r *PostgresOutboxRepository) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	executor := getExecutor(ctx, r.db)

	query := `
		UPDATE outbox
		SET retry_count = retry_count + 1,
			error_message = $1,
			status = CASE WHEN retry_count + 1 >= 5 THEN 'FAILED' ELSE 'PENDING' END
		WHERE id = $2
	`

	_, err := executor.ExecContext(ctx, query, errMsg, id)
	if err != nil {
		return fmt.Errorf("failed to mark outbox record as failed: %w", err)
	}

	return nil
}
