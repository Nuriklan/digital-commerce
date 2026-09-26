package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/config"
	"github.com/Nuriklan/digital-commerce/pkg/events"
	"github.com/segmentio/kafka-go"
)

type NotificationService struct {
	processedEvents sync.Map
}

func NewNotificationService() *NotificationService {
	return &NotificationService{}
}

func (s *NotificationService) HandleOrderEvent(ctx context.Context, msg kafka.Message) error {
	event, err := events.UnmarshalOrderCreatedEvent(msg.Value)
	if err != nil {
		// Deserialization error (malformed JSON / poison message)
		return fmt.Errorf("failed to unmarshal order event: %w", err)
	}

	if _, loaded := s.processedEvents.LoadOrStore(event.EventID, true); loaded {
		log.Printf("[Notification Service] duplicate event skipped: EventID=%s OrderID=%s",
			event.EventID, event.Payload.OrderID)
		return nil
	}

	switch event.EventType {
	case events.EventTypeOrderCreated:
		s.sendOrderCreatedNotification(event)
	default:
		log.Printf("[Notification Service] ignored unknown event type: %s", event.EventType)
	}

	return nil
}

func (s *NotificationService) sendOrderCreatedNotification(event events.OrderCreatedEvent) {
	log.Printf("==================================================")
	log.Printf("[Notification Service] 📩 SENDING NOTIFICATION:")
	log.Printf("  • EventID:  %s", event.EventID)
	log.Printf("  • OrderID:  %s", event.Payload.OrderID)
	log.Printf("  • UserID:   %s", event.Payload.UserID)
	log.Printf("  • Total:    $%.2f", event.Payload.TotalAmount)
	log.Printf("  • Items:    %d items", len(event.Payload.Items))
	for _, it := range event.Payload.Items {
		log.Printf("     - %s (x%d) @ $%.2f", it.ProductName, it.Quantity, it.Price)
	}
	log.Printf("  • Status:   Notification successfully dispatched to user!")
	log.Printf("==================================================")
}

func main() {
	cfg := config.Load()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	notifService := NewNotificationService()

	// 1. Creating a producer for the DLQ topic
	dlqProducer := events.NewProducer(cfg.Kafka.Brokers, events.TopicOrderEventsDLQ)
	defer dlqProducer.Close()

	// 2. Configuring the consumer with a 3-attempt policy and routing to the DLQ
	consumerCfg := events.ConsumerConfig{
		Brokers:      cfg.Kafka.Brokers,
		Topic:        events.TopicOrderEvents,
		GroupID:      "notification-group",
		MaxRetries:   3,
		RetryBackoff: 200 * time.Millisecond,
		DLQProducer:  dlqProducer,
		DLQTopic:     events.TopicOrderEventsDLQ,
	}

	consumer := events.NewConsumer(consumerCfg, notifService.HandleOrderEvent)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		if err := consumer.Start(ctx); err != nil {
			log.Printf("[Notification Service] consumer stopped with error: %v", err)
		}
	}()

	log.Printf("[Notification Service] started successfully with DLQ enabled, waiting for events...")

	<-ctx.Done()
	log.Printf("[Notification Service] shutting down gracefully...")

	if err := consumer.Close(); err != nil {
		log.Printf("[Notification Service] error closing consumer: %v", err)
	}

	wg.Wait()
	log.Printf("[Notification Service] stopped gracefully.")
}
