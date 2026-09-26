package service

import (
	"context"
	"fmt"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/pkg/events"
)

type KafkaOrderPublisher struct {
	producer *events.Producer
}

func NewKafkaOrderPublisher(producer *events.Producer) *KafkaOrderPublisher {
	return &KafkaOrderPublisher{
		producer: producer,
	}
}

func (p *KafkaOrderPublisher) PublishOrderCreated(ctx context.Context, order domain.Order) error {
	if p.producer == nil {
		return nil
	}

	items := make([]events.OrderItemPayload, 0, len(order.Items))
	for _, it := range order.Items {
		items = append(items, events.OrderItemPayload{
			ProductID:   it.ProductID,
			ProductName: it.Name,
			Price:       it.Price,
			Quantity:    it.Quantity,
		})
	}

	event := events.NewOrderCreatedEvent(order.ID, order.UserID, order.CalculateTotal(), items)

	payload, err := event.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal order created event: %w", err)
	}

	if err := p.producer.Publish(ctx, event.PartitionKey(), payload); err != nil {
		return fmt.Errorf("failed to publish order created event: %w", err)
	}

	return nil
}
