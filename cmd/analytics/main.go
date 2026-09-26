package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/Nuriklan/digital-commerce/internal/config"
	"github.com/Nuriklan/digital-commerce/pkg/events"
	"github.com/segmentio/kafka-go"
)

// AnalyticsService aggregates business metrics in real time based on an event stream
type AnalyticsService struct {
	mu                sync.Mutex
	totalOrders       int
	totalRevenue      float64
	productQuantities map[string]int
}

func NewAnalyticsService() *AnalyticsService {
	return &AnalyticsService{
		productQuantities: make(map[string]int),
	}
}

// HandleOrderEvent processes the order event and recalculates analytics
func (s *AnalyticsService) HandleOrderEvent(ctx context.Context, msg kafka.Message) error {
	event, err := events.UnmarshalOrderCreatedEvent(msg.Value)
	if err != nil {
		return fmt.Errorf("analytics: failed to unmarshal event: %w", err)
	}

	if event.EventType != events.EventTypeOrderCreated {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalOrders++
	s.totalRevenue += event.Payload.TotalAmount

	for _, item := range event.Payload.Items {
		s.productQuantities[item.ProductName] += item.Quantity
	}

	// Rolling out the updated analytics dashboard
	log.Printf("==================================================")
	log.Printf("[Analytics Service] 📊 REAL-TIME METRICS UPDATED:")
	log.Printf("  • Total Orders Processed: %d", s.totalOrders)
	log.Printf("  • Total Platform Revenue: $%.2f", s.totalRevenue)
	log.Printf("  • Product Sales Breakdown:")
	for name, qty := range s.productQuantities {
		log.Printf("     - %s: %d pcs", name, qty)
	}
	log.Printf("==================================================")

	return nil
}

func main() {
	cfg := config.Load()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	analyticsSvc := NewAnalyticsService()

	// IMPORTANT: Specify the unique group "analytics-group"
	// This ensures that the topic is read independently of the "notification-group"
	consumerCfg := events.ConsumerConfig{
		Brokers: cfg.Kafka.Brokers,
		Topic:   events.TopicOrderEvents,
		GroupID: "analytics-group",
	}

	consumer := events.NewConsumer(consumerCfg, analyticsSvc.HandleOrderEvent)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		if err := consumer.Start(ctx); err != nil {
			log.Printf("[Analytics Service] consumer error: %v", err)
		}
	}()

	log.Printf("[Analytics Service] started successfully (group: analytics-group), tracking metrics...")

	<-ctx.Done()
	log.Printf("[Analytics Service] shutting down gracefully...")

	if err := consumer.Close(); err != nil {
		log.Printf("[Analytics Service] error closing consumer: %v", err)
	}

	wg.Wait()
	log.Printf("[Analytics Service] stopped gracefully.")
}
