package handler

import (
	"net/http"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/middleware"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/Nuriklan/digital-commerce/pkg/auth"
)

func NewCatalogRouter(productSvc *service.ProductService, jwtMgr *auth.JWTManager) http.Handler {
	mux := http.NewServeMux()
	productH := NewProductHandler(productSvc)

	RegisterPprof(mux)

	if jwtMgr != nil {
		createProductHandler := middleware.Chain(
			http.HandlerFunc(productH.CreateProduct),
			middleware.JWTAuth(jwtMgr),
			middleware.RequireRole(domain.RoleAdmin),
		)
		mux.Handle("POST /products", createProductHandler)
	} else {
		mux.HandleFunc("POST /products", productH.CreateProduct)
	}

	mux.HandleFunc("GET /products", productH.ListProducts)
	mux.HandleFunc("GET /products/{id}", productH.GetProduct)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "catalog"})
	})

	return middleware.Chain(
		mux,
		middleware.RequestID,
		middleware.Logger,
		middleware.Recoverer,
		middleware.CORS,
	)
}

func NewOrderRouter(
	userSvc *service.UserService,
	orderSvc *service.OrderService,
	paymentSvc *service.PaymentService,
	authSvc *service.AuthService,
	jwtMgr *auth.JWTManager,
) http.Handler {
	mux := http.NewServeMux()

	userH := NewUserHandler(userSvc)
	orderH := NewOrderHandler(orderSvc)
	paymentH := NewPaymentHandler(paymentSvc)

	if authSvc != nil {
		authH := NewAuthHandler(authSvc)
		mux.HandleFunc("POST /auth/register", authH.Register)
		mux.HandleFunc("POST /auth/login", authH.Login)
	}

	mux.HandleFunc("POST /users", userH.CreateUser)
	mux.HandleFunc("GET /users/{id}", userH.GetUser)

	if jwtMgr != nil {
		createOrderHandler := middleware.Chain(
			http.HandlerFunc(orderH.CreateOrder),
			middleware.JWTAuth(jwtMgr),
		)
		mux.Handle("POST /orders", createOrderHandler)
	} else {
		mux.HandleFunc("POST /orders", orderH.CreateOrder)
	}

	mux.HandleFunc("GET /orders/{id}", orderH.GetOrder)
	mux.HandleFunc("POST /orders/{id}/cancel", orderH.CancelOrder)

	mux.HandleFunc("POST /payments", paymentH.CreatePayment)
	mux.HandleFunc("GET /payments/{id}", paymentH.GetPayment)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "order"})
	})

	return middleware.Chain(
		mux,
		middleware.RequestID,
		middleware.Logger,
		middleware.Recoverer,
		middleware.CORS,
	)
}

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

	mux.HandleFunc("POST /users", userH.CreateUser)
	mux.HandleFunc("GET /users/{id}", userH.GetUser)

	mux.HandleFunc("POST /products", productH.CreateProduct)
	mux.HandleFunc("GET /products", productH.ListProducts)
	mux.HandleFunc("GET /products/{id}", productH.GetProduct)

	mux.HandleFunc("POST /orders", orderH.CreateOrder)
	mux.HandleFunc("GET /orders/{id}", orderH.GetOrder)
	mux.HandleFunc("POST /orders/{id}/cancel", orderH.CancelOrder)

	mux.HandleFunc("POST /payments", paymentH.CreatePayment)
	mux.HandleFunc("GET /payments/{id}", paymentH.GetPayment)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return middleware.Chain(
		mux,
		middleware.RequestID,
		middleware.Logger,
		middleware.Recoverer,
	)
}
