package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/config"
	"github.com/Nuriklan/digital-commerce/internal/middleware"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()

	port := os.Getenv("GATEWAY_PORT")
	if port == "" {
		port = ":8080"
	}

	catalogURLStr := os.Getenv("CATALOG_SERVICE_URL")
	if catalogURLStr == "" {
		catalogURLStr = "http://localhost:8081"
	}

	orderURLStr := os.Getenv("ORDER_SERVICE_URL")
	if orderURLStr == "" {
		orderURLStr = "http://localhost:8082"
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 1. Connecting to Redis for Rate Limiting
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[API Gateway] Warning: Redis unavailable, rate limiter will fail open: %v", err)
	} else {
		log.Println("[API Gateway] Connected to Redis for rate limiting")
	}
	defer rdb.Close()

	// 2. Creating Reverse Proxies
	catalogProxy, err := newReverseProxy(catalogURLStr, "Catalog Service")
	if err != nil {
		log.Fatalf("[API Gateway] failed to create catalog proxy: %v", err)
	}

	orderProxy, err := newReverseProxy(orderURLStr, "Order Service")
	if err != nil {
		log.Fatalf("[API Gateway] failed to create order proxy: %v", err)
	}

	// 3. Routing
	mux := http.NewServeMux()

	// Маршруты Каталога
	mux.Handle("/products", catalogProxy)
	mux.Handle("/products/", catalogProxy)

	mux.Handle("/orders", orderProxy)
	mux.Handle("/orders/", orderProxy)
	mux.Handle("/payments", orderProxy)
	mux.Handle("/payments/", orderProxy)
	mux.Handle("/users", orderProxy)
	mux.Handle("/users/", orderProxy)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "api-gateway",
		})
	})

	// 4. Middlewares (Rate Limiter: 20 requests per second for IP)
	rateLimiter := middleware.NewRedisRateLimiter(rdb, 20, time.Second)

	handlerChain := middleware.Chain(
		mux,
		middleware.RequestID,
		middleware.RedisLimit(rateLimiter),
		middleware.Logger,
		middleware.Recoverer,
	)

	server := &http.Server{
		Addr:         port,
		Handler:      handlerChain,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		log.Printf("[API Gateway] listening on http://localhost%s", server.Addr)
		log.Printf("[API Gateway] proxying /products -> %s", catalogURLStr)
		log.Printf("[API Gateway] proxying /orders, /payments, /users -> %s", orderURLStr)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[API Gateway] error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[API Gateway] shutting down gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[API Gateway] shutdown failed: %v", err)
	}
	log.Println("[API Gateway] stopped.")
}

func newReverseProxy(targetURLStr, serviceName string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(targetURLStr)
	if err != nil {
		return nil, fmt.Errorf("invalid target url: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host

		reqID := middleware.GetRequestID(req.Context())
		if reqID != "" && reqID != "unknown" {
			req.Header.Set(middleware.HeaderXRequestID, reqID)
		}
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, proxyErr error) {
		log.Printf("[API Gateway] downstream error for %s (%s): %v", serviceName, r.URL.Path, proxyErr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "bad gateway",
			"service": serviceName,
			"details": proxyErr.Error(),
		})
	}

	return proxy, nil
}
