package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
)

func HandleOrderCreated(ctx context.Context, job Job) error {
	switch job.Name {
	case "send_notification":
		return sendNotification(ctx, job)
	case "update_analytics":
		return updateAnalytics(ctx, job)
	case "create_delivery":
		return createDelivery(ctx, job)
	default:
		return fmt.Errorf("unknown job: %s", job.Name)
	}
}

func sendNotification(ctx context.Context, job Job) error {
	order, ok := job.Payload.(domain.Order)
	if !ok {
		return fmt.Errorf("sendNotification: invalid payload type")
	}

	time.Sleep(50 * time.Millisecond)
	fmt.Printf("[notification] order %s — email sent to user %s\n", order.ID, order.UserID)
	return nil
}

func updateAnalytics(ctx context.Context, job Job) error {
	order, ok := job.Payload.(domain.Order)
	if !ok {
		return fmt.Errorf("updateAnalytics: invalid payload type")
	}

	time.Sleep(30 * time.Millisecond)
	fmt.Printf("[analytics]    order %s — total %.2f recorded\n", order.ID, order.CalculateTotal())
	return nil
}

func createDelivery(ctx context.Context, job Job) error {
	order, ok := job.Payload.(domain.Order)
	if !ok {
		return fmt.Errorf("createDelivery: invalid payload type")
	}

	time.Sleep(70 * time.Millisecond)
	fmt.Printf("[delivery]     order %s — delivery task created\n", order.ID)
	return nil
}
