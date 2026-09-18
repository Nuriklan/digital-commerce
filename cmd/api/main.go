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

	"github.com/Nuriklan/digital-commerce/internal/config"
	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/handler"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/Nuriklan/digital-commerce/internal/worker"
)

func main() {
	// 1. Configuration
	cfg := config.Load()

	// 2. OS termination signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 3. Repositories layer
	db, err := repository.NewPostgresDB(cfg.Database)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("Successfully connected to PostgreSQL")

	userRepo := repository.NewUserPostgresRepository(db)
	productRepo := repository.NewProductPostgresRepository(db)
	orderRepo := repository.NewOrderPostgresRepository(db)
	paymentRepo := repository.NewPaymentPostgresRepository(db)

	// Initial population with test products
	game1, _ := domain.NewProduct("Cyberpunk 2077", 29.99)
	game2, _ := domain.NewProduct("Witcher 3", 14.99)
	_ = productRepo.Save(game1)
	_ = productRepo.Save(game2)
	log.Printf("Seeded products: %s (%s), %s (%s)", game1.Name, game1.ID, game2.Name, game2.ID)

	// 4. Queue / workers layer
	pool := worker.NewPool(cfg.Worker.Workers, cfg.Worker.QueueSize, worker.HandleOrderCreated)
	pool.Start(ctx)

	eventsPublisher := service.NewWorkerPoolEventPublisher(pool)

	// 5. Use Cases / Business logic
	userSvc := service.NewUserService(userRepo)
	productSvc := service.NewProductService(productRepo)
	orderSvc := service.NewOrderService(orderRepo, userRepo, productRepo, eventsPublisher)
	paymentSvc := service.NewPaymentService(paymentRepo, orderRepo)

	// 6. Handlers and routing
	router := handler.NewRouter(userSvc, productSvc, orderSvc, paymentSvc)

	// 7. HTTP server configuration
	server := &http.Server{
		Addr:         cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		log.Printf("HTTP server listening on http://localhost%s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 8. Graceful shutdown
	<-ctx.Done()
	log.Println("Shutting down server gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown failed: %v", err)
	}

	log.Println("Waiting for background workers to finish...")
	pool.Wait()

	log.Println("Server exited properly.")
}
