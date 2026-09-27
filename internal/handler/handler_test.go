package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/handler"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/google/uuid"
)

type testContext struct {
	router      http.Handler
	userRepo    repository.UserRepository
	productRepo repository.ProductRepository
	orderRepo   repository.OrderRepository
	paymentRepo repository.PaymentRepository
}

func setupTestApp() *testContext {
	userRepo := repository.NewUserMemoryRepository()
	productRepo := repository.NewProductMemoryRepository()
	orderRepo := repository.NewOrderMemoryRepository()
	paymentRepo := repository.NewPaymentMemoryRepository()
	idempotencyRepo := repository.NewIdempotencyMemoryRepository()
	txManager := repository.NewMemoryTxManager()

	userSvc := service.NewUserService(userRepo)
	productSvc := service.NewProductService(productRepo)
	orderSvc := service.NewOrderService(orderRepo, userRepo, productRepo, txManager, nil)
	paymentSvc := service.NewPaymentService(paymentRepo, orderRepo, txManager, idempotencyRepo)

	router := handler.NewRouter(userSvc, productSvc, orderSvc, paymentSvc)

	return &testContext{
		router:      router,
		userRepo:    userRepo,
		productRepo: productRepo,
		orderRepo:   orderRepo,
		paymentRepo: paymentRepo,
	}
}

func TestHealthCheck(t *testing.T) {
	app := setupTestApp()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	reqID := rec.Header().Get("X-Request-ID")
	if reqID == "" {
		t.Error("expected X-Request-ID header in response")
	}
}

func TestCreateAndGetUser(t *testing.T) {
	app := setupTestApp()

	// Create the user
	body := `{"name":"Bob", "email":"bob@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var createdUser domain.User
	if err := json.Unmarshal(rec.Body.Bytes(), &createdUser); err != nil {
		t.Fatalf("failed to decode user response: %v", err)
	}

	if createdUser.Name != "Bob" || createdUser.Email != "bob@example.com" {
		t.Errorf("unexpected user data: %+v", createdUser)
	}

	// Get created user by ID
	getReq := httptest.NewRequest(http.MethodGet, "/users/"+createdUser.ID.String(), nil)
	getRec := httptest.NewRecorder()

	app.router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", getRec.Code)
	}

	// Get not existing user
	randomID := uuid.New()
	notFoundReq := httptest.NewRequest(http.MethodGet, "/users/"+randomID.String(), nil)
	notFoundRec := httptest.NewRecorder()

	app.router.ServeHTTP(notFoundRec, notFoundReq)

	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 Not Found, got %d", notFoundRec.Code)
	}
}

func TestCreateUser_ValidationErrors(t *testing.T) {
	app := setupTestApp()

	tests := []struct {
		name       string
		payload    string
		wantStatus int
	}{
		{
			name:       "empty name",
			payload:    `{"name":"","email":"test@example.com"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty email",
			payload:    `{"name":"Alice","email":""}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON syntax",
			payload:    `{"name": bad_json}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown fields",
			payload:    `{"name":"Alice","email":"alice@example.com","extra":"not allowed"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(tt.payload))
			rec := httptest.NewRecorder()

			app.router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d, body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestOrderLifecycle(t *testing.T) {
	app := setupTestApp()

	// Подготовим юзера и товар в хранилище
	user, _ := domain.NewUser("Charlie", "charlie@example.com")
	product, _ := domain.NewProduct("Steam Deck", 399.99)
	_ = app.userRepo.Save(context.Background(), user)
	_ = app.productRepo.Save(context.Background(), product)

	// 1. Creating order
	orderPayload := map[string]any{
		"user_id": user.ID.String(),
		"items": []map[string]any{
			{"product_id": product.ID.String(), "quantity": 1},
		},
	}
	bodyBytes, _ := json.Marshal(orderPayload)

	createReq := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBuffer(bodyBytes))
	createRec := httptest.NewRecorder()
	app.router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", createRec.Code, createRec.Body.String())
	}

	var order domain.Order
	_ = json.Unmarshal(createRec.Body.Bytes(), &order)

	if order.Status != domain.OrderStatusPending {
		t.Errorf("expected pending status, got %s", order.Status)
	}

	// 2. Отменяем заказ
	cancelReq := httptest.NewRequest(http.MethodPost, "/orders/"+order.ID.String()+"/cancel", nil)
	cancelRec := httptest.NewRecorder()
	app.router.ServeHTTP(cancelRec, cancelReq)

	if cancelRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on cancel, got %d", cancelRec.Code)
	}

	// 3. Повторная отмена уже отмененного заказа должна вернуть 400 Bad Request
	cancelAgainRec := httptest.NewRecorder()
	app.router.ServeHTTP(cancelAgainRec, cancelReq)

	if cancelAgainRec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request on duplicate cancel, got %d", cancelAgainRec.Code)
	}
}

func TestPayment_IdempotencyAndTransactions(t *testing.T) {
	app := setupTestApp()

	user, _ := domain.NewUser("Dave", "dave@example.com")
	product, _ := domain.NewProduct("Keyboard", 99.0)
	_ = app.userRepo.Save(context.Background(), user)
	_ = app.productRepo.Save(context.Background(), product)

	// 1. Create order
	orderPayload := map[string]any{
		"user_id": user.ID.String(),
		"items": []map[string]any{
			{"product_id": product.ID.String(), "quantity": 1},
		},
	}
	bodyBytes, _ := json.Marshal(orderPayload)
	createReq := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBuffer(bodyBytes))
	createRec := httptest.NewRecorder()
	app.router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", createRec.Code)
	}

	var order domain.Order
	_ = json.Unmarshal(createRec.Body.Bytes(), &order)

	// 2. Pay with Idempotency-Key
	idempotencyKey := "idemp-key-12345"
	paymentPayload := map[string]any{
		"order_id": order.ID.String(),
	}
	payBytes, _ := json.Marshal(paymentPayload)

	payReq1 := httptest.NewRequest(http.MethodPost, "/payments", bytes.NewBuffer(payBytes))
	payReq1.Header.Set("Idempotency-Key", idempotencyKey)
	payRec1 := httptest.NewRecorder()
	app.router.ServeHTTP(payRec1, payReq1)

	if payRec1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on first payment, got %d, body: %s", payRec1.Code, payRec1.Body.String())
	}

	var payment1 domain.Payment
	_ = json.Unmarshal(payRec1.Body.Bytes(), &payment1)

	if payment1.Status != domain.PaymentStatusSuccess {
		t.Errorf("expected payment status success, got %s", payment1.Status)
	}

	// 3. Retry payment with the exact same Idempotency-Key
	payReq2 := httptest.NewRequest(http.MethodPost, "/payments", bytes.NewBuffer(payBytes))
	payReq2.Header.Set("Idempotency-Key", idempotencyKey)
	payRec2 := httptest.NewRecorder()
	app.router.ServeHTTP(payRec2, payReq2)

	if payRec2.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on idempotent replay, got %d, body: %s", payRec2.Code, payRec2.Body.String())
	}

	var payment2 domain.Payment
	_ = json.Unmarshal(payRec2.Body.Bytes(), &payment2)

	if payment2.ID != payment1.ID {
		t.Errorf("expected same payment ID on idempotent replay: got %s, want %s", payment2.ID, payment1.ID)
	}

	// 4. Try paying the already paid order with a DIFFERENT Idempotency-Key
	payReq3 := httptest.NewRequest(http.MethodPost, "/payments", bytes.NewBuffer(payBytes))
	payReq3.Header.Set("Idempotency-Key", "different-key-999")
	payRec3 := httptest.NewRecorder()
	app.router.ServeHTTP(payRec3, payReq3)

	if payRec3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when paying already paid order, got %d, body: %s", payRec3.Code, payRec3.Body.String())
	}
}
