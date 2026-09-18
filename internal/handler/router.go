package handler

import (
	"net/http"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/middleware"
	"github.com/Nuriklan/digital-commerce/internal/service"
)

func NewRouter(
	userSvc *service.UserService,
	productSvc *service.ProductService,
	orderSvc *service.OrderService,
	paymentSvc *service.PaymentService,
) http.Handler {
	mux := http.NewServeMux()

	userH := NewUserHandler(userSvc)
	productH := NewProductHandler(productSvc)
	orderH := NewOrderHandler(orderSvc)
	paymentH := NewPaymentHandler(paymentSvc)

	// Users
	mux.HandleFunc("POST /users", userH.CreateUser)
	mux.HandleFunc("GET /users/{id}", userH.GetUser)

	// Products
	mux.HandleFunc("POST /products", productH.CreateProduct)
	mux.HandleFunc("GET /products", productH.ListProducts)
	mux.HandleFunc("GET /products/{id}", productH.GetProduct)

	// Orders
	mux.HandleFunc("POST /orders", orderH.CreateOrder)
	mux.HandleFunc("GET /orders/{id}", orderH.GetOrder)
	mux.HandleFunc("POST /orders/{id}/cancel", orderH.CancelOrder)

	// Payments
	mux.HandleFunc("POST /payments", paymentH.CreatePayment)
	mux.HandleFunc("GET /payments/{id}", paymentH.GetPayment)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	limiter := middleware.NewRateLimiter(5, 10, time.Second)

	return middleware.Chain(
		mux,
		middleware.RequestID,
		middleware.Limit(limiter),
		middleware.Logger,
		middleware.Recoverer,
	)
}
