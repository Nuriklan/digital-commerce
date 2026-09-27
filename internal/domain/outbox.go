package domain

import (
	"time"

	"github.com/google/uuid"
)

type OutboxStatus string

const (
	OutboxStatusPending   OutboxStatus = "PENDING"
	OutboxStatusPublished OutboxStatus = "PUBLISHED"
	OutboxStatusFailed    OutboxStatus = "FAILED"
)

type OutboxRecord struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	Topic         string
	Payload       []byte
	Status        OutboxStatus
	RetryCount    int
	ErrorMessage  *string
	CreatedAt     time.Time
	PublishedAt   *time.Time
}

func NewOutboxRecord(aggregateType string, aggregateID uuid.UUID, topic string, payload []byte) OutboxRecord {
	return OutboxRecord{
		ID:            uuid.New(),
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Topic:         topic,
		Payload:       payload,
		Status:        OutboxStatusPending,
		RetryCount:    0,
		CreatedAt:     time.Now().UTC(),
	}
}
