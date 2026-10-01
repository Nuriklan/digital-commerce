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
	"github.com/Nuriklan/digital-commerce/internal/handler"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/Nuriklan/digital-commerce/internal/worker"
	"github.com/Nuriklan/digital-commerce/pkg/auth"
	"github.com/Nuriklan/digital-commerce/pkg/client"
	"github.com/Nuriklan/digital-commerce/pkg/events"
)

func main() {
	cfg := config.Load()

	port := os.Getenv("ORDER_PORT")
	if port == "" {
		port = ":8082"
	}

	catalogGRPCAddr := os.Getenv("CATALOG_GRPC_ADDR")
	if catalogGRPCAddr == "" {
		catalogGRPCAddr = "localhost:50051"
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 1. PostgreSQL
	db, err := repository.NewPostgresDB(cfg.Database)
	if err != nil {
		log.Fatalf("[Order Service] failed to connect to DB: %v", err)
	}
	defer db.Close()

	// 2. Orders, payments and users repository
	txManager := repository.NewPostgresTxManager(db)
	userRepo := repository.NewUserPostgresRepository(db)
	orderRepo := repository.NewOrderPostgresRepository(db)
	paymentRepo := repository.NewPaymentPostgresRepository(db)
	idempotencyRepo := repository.NewIdempotencyPostgresRepository(db)
	outboxRepo := repository.NewPostgresOutboxRepository(db)

	// 3. gRPC Client to Catalog Service with connection lifecycle and timeout
	catalogClient, err := client.NewCatalogGRPCClient(catalogGRPCAddr, 2*time.Second)
	if err != nil {
		log.Fatalf("[Order Service] failed to initialize Catalog gRPC client: %v", err)
	}
	defer catalogClient.Close()

	// 4. Kafka Event Producer
	kafkaProducer := events.NewProducer(cfg.Kafka.Brokers, events.TopicOrderEvents)
	defer func() {
		if err := kafkaProducer.Close(); err != nil {
			log.Printf("[Order Service] error closing kafka producer: %v", err)
		}
	}()

	outboxWorker := worker.NewOutboxWorker(outboxRepo, kafkaProducer, 1*time.Second, 20)
	go outboxWorker.Start(ctx)

	// 5. Business logic
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "digital-commerce-secret-key-change-in-prod"
	}
	jwtMgr := auth.NewJWTManager(jwtSecret, 24*time.Hour)

	userSvc := service.NewUserService(userRepo)
	authSvc := service.NewAuthService(userRepo, jwtMgr)
	orderSvc := service.NewOrderService(orderRepo, userRepo, catalogClient, txManager, outboxRepo)
	paymentSvc := service.NewPaymentService(paymentRepo, orderRepo, txManager, idempotencyRepo)

	router := handler.NewOrderRouter(userSvc, orderSvc, paymentSvc, authSvc, jwtMgr)

	server := &http.Server{
		Addr:         port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		log.Printf("[Order Service] listening on http://localhost%s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[Order Service] error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[Order Service] shutting down gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Order Service] HTTP shutdown failed: %v", err)
	}

	outboxWorker.Wait()

	log.Println("[Order Service] stopped gracefully.")
}
