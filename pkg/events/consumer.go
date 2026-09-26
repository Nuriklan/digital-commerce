package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

type MessageHandler func(ctx context.Context, msg kafka.Message) error

type ConsumerConfig struct {
	Brokers      []string
	Topic        string
	GroupID      string
	MaxRetries   int
	RetryBackoff time.Duration
	DLQProducer  *Producer
	DLQTopic     string
}

type Consumer struct {
	reader       *kafka.Reader
	handler      MessageHandler
	maxRetries   int
	retryBackoff time.Duration
	dlqProducer  *Producer
	dlqTopic     string
}

func NewConsumer(cfg ConsumerConfig, handler MessageHandler) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:                cfg.Brokers,
		Topic:                  cfg.Topic,
		GroupID:                cfg.GroupID,
		MinBytes:               1,
		MaxBytes:               10e6,
		MaxWait:                500 * time.Millisecond,
		CommitInterval:         0,
		StartOffset:            kafka.FirstOffset,
		WatchPartitionChanges:  true,
		PartitionWatchInterval: 1 * time.Second,
	})

	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	backoff := cfg.RetryBackoff
	if backoff <= 0 {
		backoff = 100 * time.Millisecond
	}

	return &Consumer{
		reader:       reader,
		handler:      handler,
		maxRetries:   maxRetries,
		retryBackoff: backoff,
		dlqProducer:  cfg.DLQProducer,
		dlqTopic:     cfg.DLQTopic,
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	log.Printf("[Kafka Consumer] started listening to topic '%s' (group: '%s', max_retries: %d)",
		c.reader.Config().Topic, c.reader.Config().GroupID, c.maxRetries)

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Printf("[Kafka Consumer] context cancelled, stopping consumer loop")
				return nil
			}
			return fmt.Errorf("failed to fetch message: %w", err)
		}

		// Performing processing using a retry mechanism (retries with exponential backoff)
		var lastErr error
		attempts := 0

		for attempts < c.maxRetries {
			attempts++
			lastErr = c.handler(ctx, msg)
			if lastErr == nil {
				break
			}

			log.Printf("[Kafka Consumer] attempt %d/%d failed for offset %d: %v",
				attempts, c.maxRetries, msg.Offset, lastErr)

			if attempts < c.maxRetries {
				// Exponential backoff before the next retry
				backoffDuration := c.retryBackoff * time.Duration(1<<(attempts-1))
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(backoffDuration):
				}
			}
		}

		// If all attempts have been exhausted, move the message to the Dead-Letter Queue (DLQ)
		if lastErr != nil {
			log.Printf("[Kafka Consumer] ❌ message processing failed permanently after %d attempts (offset %d)",
				attempts, msg.Offset)

			if c.dlqProducer != nil && c.dlqTopic != "" {
				dlqMsg := DeadLetterMessage{
					OriginalTopic: msg.Topic,
					Partition:     msg.Partition,
					Offset:        msg.Offset,
					Key:           string(msg.Key),
					Value:         string(msg.Value),
					Error:         lastErr.Error(),
					FailedAt:      time.Now().UTC(),
					Attempts:      attempts,
				}

				if dlqPayload, mErr := json.Marshal(dlqMsg); mErr == nil {
					if pErr := c.dlqProducer.Publish(ctx, string(msg.Key), dlqPayload); pErr != nil {
						log.Printf("[Kafka Consumer] CRITICAL: failed to route message to DLQ: %v", pErr)
					} else {
						log.Printf("[Kafka Consumer] 📬 routed poison message to DLQ '%s' (offset %d)",
							c.dlqTopic, msg.Offset)
					}
				}
			}
		}

		// IMPORTANT: Commit the offset (both upon success and after sending to the DLQ)
		// This prevents partition stalling (head-of-line blocking)
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("[Kafka Consumer] failed to commit offset %d: %v", msg.Offset, err)
		}
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
