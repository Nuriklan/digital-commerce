package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/handler"
	"github.com/Nuriklan/digital-commerce/internal/storage"
	"github.com/Nuriklan/digital-commerce/internal/worker"
)

func main() {
	// 1. Context for intercepting OS signals (SIGINT, SIGTERM)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 2. Initialize the storage
	store := storage.NewMemoryStorage()

	// Pre-fill a couple of products to make testing easier
	game1, _ := domain.NewProduct("Cyberpunk 2077", 29.99)
	game2, _ := domain.NewProduct("Witcher 3", 14.99)
	store.SaveProduct(game1)
	store.SaveProduct(game2)
	log.Printf("Seeded products: %s (%s), %s (%s)", game1.Name, game1.ID, game2.Name, game2.ID)

	// 3. Start Worker Pool
	pool := worker.NewPool(3, 20, worker.HandleOrderCreated)
	pool.Start(ctx)

	// 4. Assembling a router with routes and middleware
	router := handler.NewRouter(store, pool)

	// 5. Configuring the HTTP server with mandatory timeouts
	server := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  5 * time.Second,  // maximum time to read the entire request
		WriteTimeout: 10 * time.Second, // maximum response recording time
		IdleTimeout:  60 * time.Second, // keep-alive connection hold time
	}

	// 6. We start the server in a separate goroutine so that `main` does not get blocked
	go func() {
		log.Printf("HTTP server listening on http://localhost%s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 7. Waiting for a completion signal from the OS
	<-ctx.Done()
	log.Println("Shutting down server gracefully...")

	// We allow the server up to 5 seconds to gracefully complete current HTTP requests
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown failed: %v", err)
	}

	// We wait for the background workers to finish
	log.Println("Waiting for background workers to finish...")
	pool.Wait()

	log.Println("Server exited properly.")
}
