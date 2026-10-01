package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/handler"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/Nuriklan/digital-commerce/pkg/auth"
)

func TestSecurity_RBAC_And_Authentication_E2E(t *testing.T) {
	jwtMgr := auth.NewJWTManager("e2e-test-jwt-secret-key-12345", 1*time.Hour)

	userRepo := repository.NewUserMemoryRepository()
	productRepo := repository.NewProductMemoryRepository()
	orderRepo := repository.NewOrderMemoryRepository()
	paymentRepo := repository.NewPaymentMemoryRepository()
	idempotencyRepo := repository.NewIdempotencyMemoryRepository()
	txManager := repository.NewMemoryTxManager()

	userSvc := service.NewUserService(userRepo)
	authSvc := service.NewAuthService(userRepo, jwtMgr)
	productSvc := service.NewProductService(productRepo)
	orderSvc := service.NewOrderService(orderRepo, userRepo, productRepo, txManager, nil)
	paymentSvc := service.NewPaymentService(paymentRepo, orderRepo, txManager, idempotencyRepo)

	catalogRouter := handler.NewCatalogRouter(productSvc, jwtMgr)
	orderRouter := handler.NewOrderRouter(userSvc, orderSvc, paymentSvc, authSvc, jwtMgr)

	// 1. Register regular USER
	regUserBody := map[string]string{
		"name":     "Regular User",
		"email":    "user@example.com",
		"password": "password123",
		"role":     "USER",
	}
	body, _ := json.Marshal(regUserBody)
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	orderRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to register user: %d, body: %s", rec.Code, rec.Body.String())
	}

	var userAuthResp handler.AuthResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &userAuthResp)
	userToken := userAuthResp.Token

	// 2. Register ADMIN
	regAdminBody := map[string]string{
		"name":     "Admin User",
		"email":    "admin@example.com",
		"password": "adminpassword123",
		"role":     "ADMIN",
	}
	body, _ = json.Marshal(regAdminBody)
	req = httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	orderRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to register admin: %d", rec.Code)
	}

	var adminAuthResp handler.AuthResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &adminAuthResp)
	adminToken := adminAuthResp.Token

	// 3. Test: Unauthorized request of product creating -> 401 Unauthorized
	newProdBody := map[string]any{"name": "Gaming Laptop", "price": 1200.0}
	body, _ = json.Marshal(newProdBody)
	req = httptest.NewRequest(http.MethodPost, "/products", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	catalogRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized without token, got %d", rec.Code)
	}

	// 4. Test: Regular user tries to create product -> 403 Forbidden
	req = httptest.NewRequest(http.MethodPost, "/products", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec = httptest.NewRecorder()
	catalogRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for USER role, got %d", rec.Code)
	}

	// 5. Test: Admin creates product -> 201 Created
	req = httptest.NewRequest(http.MethodPost, "/products", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	catalogRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for ADMIN role, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var createdProduct domain.Product
	_ = json.Unmarshal(rec.Body.Bytes(), &createdProduct)

	// 6. Test: Creating order without token -> 401 Unauthorized
	createOrderReq := map[string]any{
		"items": []map[string]any{
			{"product_id": createdProduct.ID, "quantity": 1},
		},
	}
	body, _ = json.Marshal(createOrderReq)
	req = httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	orderRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated order creation, got %d", rec.Code)
	}

	// 7. Test: Creating order with valid user token -> 201 Created
	req = httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec = httptest.NewRecorder()
	orderRouter.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for authenticated order creation, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var createdOrder domain.Order
	_ = json.Unmarshal(rec.Body.Bytes(), &createdOrder)

	if createdOrder.UserID != userAuthResp.User.ID {
		t.Errorf("order must be bound to authenticated user ID: expected %s, got %s", userAuthResp.User.ID, createdOrder.UserID)
	}
}
