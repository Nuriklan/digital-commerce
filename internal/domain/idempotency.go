package domain

import (
	"time"

	"github.com/google/uuid"
)

type IdempotencyRecord struct {
	Key          string    `json:"key"`
	PaymentID    uuid.UUID `json:"payment_id"`
	OrderID      uuid.UUID `json:"order_id"`
	StatusCode   int       `json:"status_code"`
	ResponseBody []byte    `json:"response_body"`
	CreatedAt    time.Time `json:"created_at"`
}
