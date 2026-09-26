package events

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafka.Writer
}

func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Topic:                  topic,
			Balancer:               &kafka.Hash{},
			WriteTimeout:           10 * time.Second,
			ReadTimeout:            10 * time.Second,
			RequiredAcks:           kafka.RequireOne,
			BatchTimeout:           10 * time.Millisecond,
			AllowAutoTopicCreation: true,
			// Writer internal retries during metadata updates and topic creation
			MaxAttempts:     5,
			WriteBackoffMin: 100 * time.Millisecond,
			WriteBackoffMax: 1 * time.Second,
		},
	}
}

func (p *Producer) Publish(ctx context.Context, key string, payload []byte) error {
	msg := kafka.Message{
		Key:   []byte(key),
		Value: payload,
		Time:  time.Now(),
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("failed to write message to kafka: %w", err)
	}

	return nil
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
