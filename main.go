package main

import (
	"fmt"
	"log"

	"github.com/Nuriklan/digital-commerce/internal/domain"
)

func main() {
	// Create user
	user, err := domain.NewUser("Alice", "alice@example.com")
	if err != nil {
		log.Fatal(err)
	}

	// Create products
	game, err := domain.NewProduct("Cyberpunk 2077", 29.99)
	if err != nil {
		log.Fatal(err)
	}

	dlc, err := domain.NewProduct("DLC Pack", 9.99)
	if err != nil {
		log.Fatal(err)
	}

	// Create order
	order := domain.NewOrder(user.ID)

	// Add items
	order.AddItem(game, 1)
	order.AddItem(dlc, 2)

	fmt.Printf("Order total: %.2f\n", order.CalculateTotal())

	// Remove DLC
	if err := order.RemoveItem(dlc.ID); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Order total after remove: %.2f\n", order.CalculateTotal())

	// Payment
	payment, err := domain.NewPayment(order.ID, order.CalculateTotal())
	if err != nil {
		log.Fatal(err)
	}

	if err := order.Pay(); err != nil {
		log.Fatal(err)
	}

	if err := payment.MarkSuccess(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Order status: %s\n", order.Status)
	fmt.Printf("Payment status: %s\n", payment.Status)

	// Trying to cancel paid order
	if err := order.Cancel(); err != nil {
		fmt.Println("Cancel error:", err)
	}
}
