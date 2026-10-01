package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/handler"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
)

func setupCatalogBenchmarkApp(b *testing.B, count int) http.Handler {
	b.Helper()

	productRepo := repository.NewProductMemoryRepository()
	ctx := context.Background()

	for i := 0; i < count; i++ {
		p, err := domain.NewProduct(fmt.Sprintf("Benchmark Product #%d", i), float64(10+i))
		if err != nil {
			b.Fatalf("failed to create benchmark product: %v", err)
		}
		if err := productRepo.Save(ctx, p); err != nil {
			b.Fatalf("failed to save benchmark product: %v", err)
		}
	}

	productSvc := service.NewProductService(productRepo)
	return handler.NewCatalogRouter(productSvc, nil)
}

func BenchmarkListProductsHandler(b *testing.B) {
	router := setupCatalogBenchmarkApp(b, 100)
	req := httptest.NewRequest(http.MethodGet, "/products?limit=50&offset=0", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d", rec.Code)
		}
	}
}

func BenchmarkProductJSONEncoding(b *testing.B) {
	products := make([]domain.Product, 50)
	for i := 0; i < 50; i++ {
		p, _ := domain.NewProduct(fmt.Sprintf("Item %d", i), 99.99)
		products[i] = p
	}

	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := json.NewEncoder(&buf).Encode(products); err != nil {
			b.Fatalf("failed to encode json: %v", err)
		}
	}
}
