package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/config"
	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/handler"
	grpchandler "github.com/Nuriklan/digital-commerce/internal/handler/grpc"
	"github.com/Nuriklan/digital-commerce/internal/middleware"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/Nuriklan/digital-commerce/pkg/auth"
	catalogpb "github.com/Nuriklan/digital-commerce/proto/catalog"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Load()

	httpPort := os.Getenv("CATALOG_PORT")
	if httpPort == "" {
		httpPort = ":8081"
	}

	grpcPort := os.Getenv("CATALOG_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = ":50051"
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 1. PostgreSQL
	db, err := repository.NewPostgresDB(cfg.Database)
	if err != nil {
		log.Fatalf("[Catalog Service] failed to connect to DB: %v", err)
	}
	defer db.Close()

	// 2. Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("[Catalog Service] failed to connect to Redis: %v", err)
	}
	defer rdb.Close()

	// 3. Repositories and cache
	baseProductRepo := repository.NewProductPostgresRepository(db)
	productRepo := repository.NewCachedProductRepository(baseProductRepo, rdb, cfg.Redis.ProductTTL)

	// Initial data
	seedProducts(ctx, productRepo)

	// 4. Service and HTTP-handlers
	productSvc := service.NewProductService(productRepo)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "digital-commerce-secret-key-change-in-prod"
	}
	jwtMgr := auth.NewJWTManager(jwtSecret, 24*time.Hour)

	// 5. HTTP Server (REST API)
	router := handler.NewCatalogRouter(productSvc, jwtMgr)
	httpServer := &http.Server{
		Addr:         httpPort,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		log.Printf("[Catalog Service] listening on http://localhost%s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[Catalog Service] error: %v", err)
		}
	}()

	// 6. gRPC Server
	lis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		log.Fatalf("[Catalog Service] failed to listen TCP port: %s %v", grpcPort, err)
	}

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.RecoveryUnaryServerInterceptor,
			middleware.LoggingUnaryServerInterceptor,
		),
	)
	catalogGRPCServer := grpchandler.NewCatalogGRPCServer(productSvc)
	catalogpb.RegisterCatalogServiceServer(grpcServer, catalogGRPCServer)

	go func() {
		log.Printf("[Catalog Service] gRPC listening on %s", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("[Catalog Service] gRPC error: %v", err)
		}
	}()

	// 7. Graceful Shutdown
	<-ctx.Done()
	log.Println("[Catalog Service] shutting down gracefully...")

	grpcServer.GracefulStop()
	log.Println("[Catalog Service] gRPC server stopped.")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Catalog Service] HTTP shutdown failed: %v", err)
	}
	log.Println("[Catalog Service] HTTP server stopped.")
}

func seedProducts(ctx context.Context, repo repository.ProductRepository) {
	game1, _ := domain.NewProduct("Cyberpunk 2077", 29.99)
	game2, _ := domain.NewProduct("Witcher 3", 14.99)
	_ = repo.Save(ctx, game1)
	_ = repo.Save(ctx, game2)
	log.Printf("[Catalog Service] Seeded products: %s (%s), %s (%s)", game1.Name, game1.ID, game2.Name, game2.ID)
}
