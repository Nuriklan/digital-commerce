package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Topics and events contstants
const (
	TopicOrderEvents    = "order.events"
	TopicOrderEventsDLQ = "order.events.dlq"

	EventTypeOrderCreated = "order.created"
)

type OrderItemPayload struct {
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       float64   `json:"price"`
	Quantity    int       `json:"quantity"`
}

type OrderCreatedPayload struct {
	OrderID     uuid.UUID          `json:"order_id"`
	UserID      uuid.UUID          `json:"user_id"`
	TotalAmount float64            `json:"total_amount"`
	Items       []OrderItemPayload `json:"items"`
}

// Event Envelope
type OrderCreatedEvent struct {
	EventID   string              `json:"event_id"`
	EventType string              `json:"event_type"`
	Timestamp time.Time           `json:"timestamp"`
	Payload   OrderCreatedPayload `json:"payload"`
}

type DeadLetterMessage struct {
	OriginalTopic string    `json:"original_topic"`
	Partition     int       `json:"partition"`
	Offset        int64     `json:"offset"`
	Key           string    `json:"key"`
	Value         string    `json:"value"`
	Error         string    `json:"error"`
	FailedAt      time.Time `json:"failed_at"`
	Attempts      int       `json:"attempts"`
}

func (e OrderCreatedEvent) PartitionKey() string {
	return e.Payload.OrderID.String()
}

func NewOrderCreatedEvent(orderId, userId uuid.UUID, totalAmount float64, items []OrderItemPayload) OrderCreatedEvent {
	return OrderCreatedEvent{
		EventID:   uuid.New().String(),
		EventType: EventTypeOrderCreated,
		Timestamp: time.Now().UTC(),
		Payload: OrderCreatedPayload{
			OrderID:     orderId,
			UserID:      userId,
			TotalAmount: totalAmount,
			Items:       items,
		},
	}
}

func (e OrderCreatedEvent) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func UnmarshalOrderCreatedEvent(data []byte) (OrderCreatedEvent, error) {
	var event OrderCreatedEvent
	err := json.Unmarshal(data, &event)
	return event, err
}
