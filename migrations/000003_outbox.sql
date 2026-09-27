-- Migration 000003: Transactional Outbox table
-- Ensures the atomicity of business data changes and event publication

CREATE TABLE IF NOT EXISTS outbox (
    id UUID PRIMARY KEY,
    aggregate_type VARCHAR(64) NOT NULL,    -- For example: 'order', 'payment'
    aggregate_id UUID NOT NULL,             -- ID of the entity to which the event relates (order_id)
    topic VARCHAR(128) NOT NULL,            -- Destination Kafka topic (e.g., 'order.events')
    payload JSONB NOT NULL,                 -- Message body in JSON format
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING', -- PENDING, PUBLISHED, FAILED
    retry_count INT NOT NULL DEFAULT 0,     -- Number of failed publication attempts
    error_message TEXT,                     -- Error text from the last failed attempt
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ                -- Time of successful submission to the broker
);

-- Index for high-performance worker polling:
-- The worker looks only for unprocessed records with the status PENDING
CREATE INDEX IF NOT EXISTS idx_outbox_pending_polling
ON outbox (status, created_at)
WHERE status = 'PENDING';

-- Index for searching events by aggregate (for auditing and debugging)
CREATE INDEX IF NOT EXISTS idx_outbox_aggregate
ON outbox (aggregate_type, aggregate_id);
