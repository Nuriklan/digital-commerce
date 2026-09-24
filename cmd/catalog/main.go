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
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()

	port := os.Getenv("CATALOG_PORT")
	if port == "" {
		port = ":8081"
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
	router := handler.NewCatalogRouter(productSvc)

	server := &http.Server{
		Addr:         port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		log.Printf("[Catalog Service] listening on http://localhost%s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[Catalog Service] error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[Catalog Service] shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Catalog Service] shutdown failed: %v", err)
	}
	log.Println("[Catalog Service] stopped gracefully.")
}

func seedProducts(ctx context.Context, repo repository.ProductRepository) {
	game1, _ := domain.NewProduct("Cyberpunk 2077", 29.99)
	game2, _ := domain.NewProduct("Witcher 3", 14.99)
	_ = repo.Save(ctx, game1)
	_ = repo.Save(ctx, game2)
	log.Printf("[Catalog Service] Seeded products: %s (%s), %s (%s)", game1.Name, game1.ID, game2.Name, game2.ID)
}
