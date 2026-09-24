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
	"github.com/Nuriklan/digital-commerce/internal/middleware"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/Nuriklan/digital-commerce/internal/worker"
	"github.com/redis/go-redis/v9"
)

func main() {
	// 1. Configuration
	cfg := config.Load()

	// 2. OS termination signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 3. PostgreSQL connection
	db, err := repository.NewPostgresDB(cfg.Database)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("Successfully connected to PostgreSQL")

	// 4. Redis connection
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect to Redis: %v", err)
	}
	defer rdb.Close()
	log.Println("Successfully connected to Redis")

	// 5. Repositories (Decorating productRepo with Cache)
	txManager := repository.NewPostgresTxManager(db)
	userRepo := repository.NewUserPostgresRepository(db)
	baseProductRepo := repository.NewProductPostgresRepository(db)
	productRepo := repository.NewCachedProductRepository(baseProductRepo, rdb, cfg.Redis.ProductTTL)
	orderRepo := repository.NewOrderPostgresRepository(db)
	paymentRepo := repository.NewPaymentPostgresRepository(db)
	idempotencyRepo := repository.NewIdempotencyPostgresRepository(db)

	// Initial population with test products
	game1, _ := domain.NewProduct("Cyberpunk 2077", 29.99)
	game2, _ := domain.NewProduct("Witcher 3", 14.99)
	_ = productRepo.Save(ctx, game1)
	_ = productRepo.Save(ctx, game2)
	log.Printf("Seeded products: %s (%s), %s (%s)", game1.Name, game1.ID, game2.Name, game2.ID)

	// 6. Queue / workers layer
	pool := worker.NewPool(cfg.Worker.Workers, cfg.Worker.QueueSize, worker.HandleOrderCreated)
	pool.Start(ctx)

	eventsPublisher := service.NewWorkerPoolEventPublisher(pool)

	// 7. Use Cases / Business logic
	userSvc := service.NewUserService(userRepo)
	productSvc := service.NewProductService(productRepo)
	orderSvc := service.NewOrderService(orderRepo, userRepo, productRepo, eventsPublisher)
	paymentSvc := service.NewPaymentService(paymentRepo, orderRepo, txManager, idempotencyRepo)

	rateLimiter := middleware.NewRedisRateLimiter(rdb, 10, time.Second)

	// 8. Handlers and routing
	router := handler.NewRouter(userSvc, productSvc, orderSvc, paymentSvc, rateLimiter)

	// 9. HTTP server configuration
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

	// 10. Graceful shutdown
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
