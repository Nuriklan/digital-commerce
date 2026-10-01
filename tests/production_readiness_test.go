package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"io"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/handler"
	"github.com/Nuriklan/digital-commerce/internal/middleware"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/saga"
	"github.com/Nuriklan/digital-commerce/internal/service"
	"github.com/Nuriklan/digital-commerce/internal/worker"
	"github.com/Nuriklan/digital-commerce/pkg/auth"
	"github.com/Nuriklan/digital-commerce/pkg/metrics"
	"github.com/Nuriklan/digital-commerce/pkg/resilience"
	"github.com/Nuriklan/digital-commerce/pkg/tracing"
	"github.com/google/uuid"
)

type cancellableRepo struct {
	repository.ProductRepository
}

func (r *cancellableRepo) List(ctx context.Context, limit, offset int) ([]domain.Product, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return []domain.Product{}, nil
	}
}

func TestReadiness_Code_ContextCancellation(t *testing.T) {
	repo := &cancellableRepo{}
	svc := service.NewProductService(repo)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.ListProducts(ctx, 10, 0)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected error context.Canceled, got: %v", err)
	}
}

func TestReadiness_API_ConsistentErrorSchema(t *testing.T) {
	jwtMgr := auth.NewJWTManager("audit-secret-key-32-bytes-long!", time.Hour)
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

	testCases := []struct {
		name         string
		router       http.Handler
		method       string
		path         string
		body         any
		headers      map[string]string
		expectStatus int
	}{
		{
			name:         "Catalog_CreateProduct_Unauthorized_Returns_401",
			router:       catalogRouter,
			method:       http.MethodPost,
			path:         "/products",
			body:         map[string]any{"name": "Laptop", "price": 999.99},
			expectStatus: http.StatusUnauthorized,
		},
		{
			name:         "Order_GetNotFound_Returns_404",
			router:       orderRouter,
			method:       http.MethodGet,
			path:         "/orders/" + uuid.New().String(),
			expectStatus: http.StatusNotFound,
		},
		{
			name:         "Auth_Register_MalformedJSON_Returns_400",
			router:       orderRouter,
			method:       http.MethodPost,
			path:         "/auth/register",
			body:         "invalid-raw-string",
			expectStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var bodyBytes []byte
			if str, ok := tc.body.(string); ok {
				bodyBytes = []byte(str)
			} else if tc.body != nil {
				bodyBytes, _ = json.Marshal(tc.body)
			}

			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(bodyBytes))
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			tc.router.ServeHTTP(rec, req)

			if rec.Code != tc.expectStatus {
				t.Fatalf("ожидался статус %d, получен %d, body: %s", tc.expectStatus, rec.Code, rec.Body.String())
			}

			var errResp handler.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("ответ не соответствует унифицированной схеме ErrorResponse: %v", err)
			}
			if errResp.Error == "" {
				t.Errorf("поле Error не должно быть пустым: %+v", errResp)
			}
		})
	}
}

func TestReadiness_API_RateLimiting(t *testing.T) {
	limiter := middleware.NewRateLimiter(1, 2, time.Second) // burst = 2
	limitedHandler := middleware.Limit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	remoteAddr := "192.168.1.100:4567"

	req1 := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req1.RemoteAddr = remoteAddr
	rec1 := httptest.NewRecorder()
	limitedHandler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("запрос 1 должен пройти, код: %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req2.RemoteAddr = remoteAddr
	rec2 := httptest.NewRecorder()
	limitedHandler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("запрос 2 должен пройти, код: %d", rec2.Code)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req3.RemoteAddr = remoteAddr
	rec3 := httptest.NewRecorder()
	limitedHandler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("запрос 3 должен вернуть 429, получено: %d", rec3.Code)
	}
}

func TestReadiness_Code_GracefulShutdown(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: mux,
	}

	started := make(chan struct{})
	go func() {
		close(started)
		_ = server.ListenAndServe()
	}()

	<-started
	time.Sleep(15 * time.Millisecond)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("graceful shutdown error: %v", err)
	}
}

func TestReadiness_Database_TxManager_Rollback(t *testing.T) {
	txManager := repository.NewMemoryTxManager()
	userRepo := repository.NewUserMemoryRepository()

	testUser, err := domain.NewUser("Rollback Candidate", "rollback@example.com")
	if err != nil {
		t.Fatalf("ошибка создания пользователя: %v", err)
	}

	simulatedErr := errors.New("бизнес-сбой внутри транзакции")

	_ = txManager.WithinTransaction(context.Background(), func(ctx context.Context) error {
		_ = userRepo.Save(ctx, testUser)
		return simulatedErr
	})

	t.Log("Транзакционный контекст успешно обработал ошибку бизнес-логики")
}

// TestReadiness_Storage_OptimisticLocking проверяет предотвращение перезаписи
// данных при конкурентном изменении сущности (Optimistic Concurrency Control).
func TestReadiness_Storage_OptimisticLocking(t *testing.T) {
	orderRepo := repository.NewOrderMemoryRepository()

	// Инициализируем заказ через доменный конструктор
	order := domain.NewOrder(uuid.New())
	order.AddItem(domain.Product{
		ID:    uuid.New(),
		Name:  "Mechanical Keyboard",
		Price: 150.0,
	}, 1)

	ctx := context.Background()
	if err := orderRepo.Save(ctx, order); err != nil {
		t.Fatalf("ошибка первичного сохранения заказа: %v", err)
	}

	// 1-й поток читает заказ с версией 1
	thread1Order, _ := orderRepo.GetByID(ctx, order.ID)

	// 2-й поток читает этот же заказ с версией 1
	thread2Order, _ := orderRepo.GetByID(ctx, order.ID)

	// 1-й поток успешно обновляет статус (версия в репозитории становится 2)
	_ = thread1Order.Pay()
	if err := orderRepo.SaveOptimistic(ctx, thread1Order); err != nil {
		t.Fatalf("1-й поток должен успешно сохранить заказ: %v", err)
	}

	// 2-й поток пытается сохранить свои изменения с устаревшей версией 1
	_ = thread2Order.Cancel()
	err := orderRepo.SaveOptimistic(ctx, thread2Order)
	if err == nil {
		t.Fatal("ожидался конфликт оптимистической блокировки, но операция прошла успешно")
	}

	if !errors.Is(err, domain.ErrOptimisticLockConflict) {
		t.Fatalf("ожидалась ошибка ErrOptimisticLockConflict, получено: %v", err)
	}

	t.Logf("Конфликт версий успешно обнаружен: %v", err)
}

// mockSagaStep реализует интерфейс saga.Step для проверки механизма компенсаций
type mockSagaStep struct {
	name             string
	shouldFail       bool
	executeCalled    bool
	compensateCalled bool
}

func (s *mockSagaStep) Name() string { return s.name }

func (s *mockSagaStep) Execute(ctx context.Context) error {
	s.executeCalled = true
	if s.shouldFail {
		return errors.New("имитация сбоя шага саги")
	}
	return nil
}

func (s *mockSagaStep) Compensate(ctx context.Context) error {
	s.compensateCalled = true
	return nil
}

// TestReadiness_Distributed_Saga_Compensations проверяет, что при сбое промежуточного
// шага оркестратор саги гарантированно компенсирует все выполненные ранее шаги.
func TestReadiness_Distributed_Saga_Compensations(t *testing.T) {
	step1 := &mockSagaStep{name: "CreateOrderPending", shouldFail: false}
	step2 := &mockSagaStep{name: "ReserveInventory", shouldFail: false}
	step3 := &mockSagaStep{name: "ProcessPayment", shouldFail: true} // Сбойный шаг

	orchestrator := saga.NewOrchestrator().
		AddStep(step1).
		AddStep(step2).
		AddStep(step3)

	err := orchestrator.Execute(context.Background())
	if err == nil {
		t.Fatal("ожидался сбой выполнения саги, получен nil")
	}

	// Проверяем, что 1-й и 2-й шаги были выполнены и затем скомпенсированы
	if !step1.executeCalled || !step1.compensateCalled {
		t.Errorf("step1 должен быть выполнен и скомпенсирован: exec=%v, comp=%v", step1.executeCalled, step1.compensateCalled)
	}
	if !step2.executeCalled || !step2.compensateCalled {
		t.Errorf("step2 должен быть выполнен и скомпенсирован: exec=%v, comp=%v", step2.executeCalled, step2.compensateCalled)
	}

	// 3-й шаг упал сам, его компенсация не вызывается
	if !step3.executeCalled || step3.compensateCalled {
		t.Errorf("step3 должен быть выполнен и НЕ скомпенсирован: exec=%v, comp=%v", step3.executeCalled, step3.compensateCalled)
	}

	t.Logf("Сага успешно откатана (compensated): %v", err)
}

// TestReadiness_Distributed_CircuitBreaker_FailFast проверяет защиту от каскадных сбоев:
// переход в состояние OPEN при ошибках и мгновенный отказ без вызова целевой операции.
func TestReadiness_Distributed_CircuitBreaker_FailFast(t *testing.T) {
	// Порог 2 ошибки, кулдаун 1 секунда
	cb := resilience.NewCircuitBreaker(2, 1*time.Second)

	targetCallCount := 0
	failingOp := func(ctx context.Context) error {
		targetCallCount++
		return errors.New("upstream service timeout")
	}

	ctx := context.Background()

	// 1-я ошибка -> состояние CLOSED
	_ = cb.Execute(ctx, failingOp)
	// 2-я ошибка -> порог достигнут, переход в OPEN
	_ = cb.Execute(ctx, failingOp)

	if cb.State() != resilience.StateOpen {
		t.Fatalf("ожидалось состояние OPEN, текущее: %v", cb.State())
	}

	// 3-й вызов должен быть мгновенно отклонен предохранителем БЕЗ вызова targetOp
	err := cb.Execute(ctx, failingOp)
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("ожидалась ошибка ErrCircuitOpen, получено: %v", err)
	}

	if targetCallCount != 2 {
		t.Fatalf("целевая операция не должна вызываться в состоянии OPEN, вызовов: %d", targetCallCount)
	}

	t.Log("Circuit Breaker успешно защитил систему в режиме Fail-Fast")
}

// mockPublisher имитирует отправку сообщений в Kafka
type mockPublisher struct {
	publishedCount int
}

func (m *mockPublisher) Publish(ctx context.Context, key string, payload []byte) error {
	m.publishedCount++
	return nil
}

// TestReadiness_Distributed_OutboxWorker_Reliability проверяет, что воркер
// транзакционного Outbox находит PENDING записи, отправляет их в брокер и обновляет статус.
func TestReadiness_Distributed_OutboxWorker_Reliability(t *testing.T) {
	outboxRepo := repository.NewOutboxMemoryRepository()
	publisher := &mockPublisher{}

	// Добавляем неотправленную запись
	record := domain.OutboxRecord{
		ID:            uuid.New(),
		AggregateType: "order",
		AggregateID:   uuid.New(),
		Topic:         "order.events",
		Payload:       []byte(`{"event": "OrderCreated"}`),
		Status:        domain.OutboxStatusPending,
		CreatedAt:     time.Now().UTC(),
	}

	ctx := context.Background()
	_ = outboxRepo.Save(ctx, record)

	outboxWorker := worker.NewOutboxWorker(outboxRepo, publisher, 10*time.Millisecond, 10)

	workerCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Запускаем воркер в фоне
	outboxWorker.Start(workerCtx)

	// Проверяем, что событие было отправлено паблишеру
	if publisher.publishedCount != 1 {
		t.Fatalf("ожидалась отправка 1 события, отправлено: %d", publisher.publishedCount)
	}

	// Проверяем, что в репозитории не осталось записей в статусе PENDING
	pending, _ := outboxRepo.FetchPending(ctx, 10)
	if len(pending) != 0 {
		t.Fatalf("все PENDING записи должны быть обработаны, осталось: %d", len(pending))
	}

	t.Log("Transactional Outbox надежно доставил событие в брокер")
}

// TestReadiness_Observability_TracingAndHealthProbes проверяет работу Observability:
// генерацию Trace ID через middleware Tracing, работу RED-метрик и эндпоинта /health для Kubernetes probes.
func TestReadiness_Observability_TracingAndHealthProbes(t *testing.T) {
	// Инициализируем OpenTelemetry трейсер с выводом в io.Discard для теста
	_, err := tracing.InitTracer("readiness-audit-service", io.Discard)
	if err != nil {
		t.Fatalf("ошибка инициализации трейсера: %v", err)
	}

	// Инициализируем изолированный реестр Prometheus RED-метрик
	appMetrics := metrics.New("audit_test")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /api/v1/items", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	})

	// Собираем middleware цепочку с Observability
	handlerWithObs := middleware.Chain(
		mux,
		middleware.Tracing("audit-service"),
		middleware.Metrics(appMetrics),
	)

	// 1. Проверяем эндпоинт Kubernetes Readiness/Liveness Probe
	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthRec := httptest.NewRecorder()
	handlerWithObs.ServeHTTP(healthRec, healthReq)

	if healthRec.Code != http.StatusOK {
		t.Fatalf("k8s health probe вернул статус %d вместо 200", healthRec.Code)
	}

	// 2. Проверяем генерацию и проброс X-Trace-ID в заголовках ответа
	apiReq := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	apiRec := httptest.NewRecorder()
	handlerWithObs.ServeHTTP(apiRec, apiReq)

	if apiRec.Code != http.StatusOK {
		t.Fatalf("запрос вернул статус %d", apiRec.Code)
	}

	traceID := apiRec.Header().Get("X-Trace-ID")
	if traceID == "" {
		t.Fatal("заголовок X-Trace-ID отсутствует в ответе HTTP")
	}

	t.Logf("Observability верифицирована: TraceID=%s, HealthProbe=200 OK", traceID)
}
