-- Migration 000002: Add transactions, locking, and idempotency

-- Add version column to orders for optimistic locking demonstration
ALTER TABLE orders ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;

-- Table to store idempotency keys for request deduplication
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key VARCHAR(255) PRIMARY KEY,
    payment_id UUID NOT NULL,
    order_id UUID NOT NULL,
    status_code INT NOT NULL,
    response_body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_idempotency_order_id ON idempotency_keys(order_id);
