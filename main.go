package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/worker"
)

func main() {
	// graceful shutdown: catching SIGINT / SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// creating worker pool: 3 goroutines, buffer 10
	pool := worker.NewPool(3, 10, worker.HandleOrderCreated)
	pool.Start(ctx)

	// --- domain logic ---
	user, err := domain.NewUser("Alice", "alice@example.com")
	if err != nil {
		log.Fatal(err)
	}

	game, err := domain.NewProduct("Cyberpunk 2077", 29.99)
	if err != nil {
		log.Fatal(err)
	}

	order := domain.NewOrder(user.ID)
	order.AddItem(game, 1)

	if err := order.Pay(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Order %s created, status: %s\n\n", order.ID, order.Status)

	// --- sending background tasks ---
	jobs := []string{"send_notification", "update_analytics", "create_delivery"}
	for _, name := range jobs {
		ok := pool.Submit(ctx, worker.Job{Name: name, Payload: order})
		if !ok {
			log.Printf("failed to submit job %q: context cancelled", name)
		}
	}

	// waiting workers to end
	pool.Wait()
	fmt.Println("\ndone.")
}
