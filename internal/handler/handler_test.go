package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/handler"
	"github.com/Nuriklan/digital-commerce/internal/storage"
	"github.com/google/uuid"
)

func setupRouter() (http.Handler, *storage.MemoryStorage) {
	store := storage.NewMemoryStorage()

	router := handler.NewRouter(store, nil)
	return router, store
}

func TestHealthCheck(t *testing.T) {
	router, _ := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	reqID := rec.Header().Get("X-Request-ID")
	if reqID == "" {
		t.Error("expected X-Request-ID header in response")
	}
}

func TestCreateAndGetUser(t *testing.T) {
	router, _ := setupRouter()

	// Create the user
	body := `{"name":"Bob", "email":"bob@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

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

	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", getRec.Code)
	}

	// Get not existing user
	randomID := uuid.New()
	notFoundReq := httptest.NewRequest(http.MethodGet, "/users/"+randomID.String(), nil)
	notFoundRec := httptest.NewRecorder()

	router.ServeHTTP(notFoundRec, notFoundReq)

	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 Not Found, got %d", notFoundRec.Code)
	}
}

func TestCreateUser_ValidationErrors(t *testing.T) {
	router, _ := setupRouter()

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

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d, body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestOrderLifecycle(t *testing.T) {
	router, store := setupRouter()

	// Подготовим юзера и товар в хранилище
	user, _ := domain.NewUser("Charlie", "charlie@example.com")
	product, _ := domain.NewProduct("Steam Deck", 399.99)
	store.SaveUser(user)
	store.SaveProduct(product)

	// 1. Создаем заказ через HTTP API
	orderPayload := map[string]any{
		"user_id": user.ID.String(),
		"items": []map[string]any{
			{"product_id": product.ID.String(), "quantity": 1},
		},
	}
	bodyBytes, _ := json.Marshal(orderPayload)

	createReq := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBuffer(bodyBytes))
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

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
	router.ServeHTTP(cancelRec, cancelReq)

	if cancelRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on cancel, got %d", cancelRec.Code)
	}

	// 3. Повторная отмена уже отмененного заказа должна вернуть 400 Bad Request
	cancelAgainRec := httptest.NewRecorder()
	router.ServeHTTP(cancelAgainRec, cancelReq)

	if cancelAgainRec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request on duplicate cancel, got %d", cancelAgainRec.Code)
	}
}
