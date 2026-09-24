package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrProductNotFound    = errors.New("product not found in catalog")
	ErrServiceUnavailable = errors.New("catalog  service unavaliable")
)

type CatalogHTTPClient struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
	retryDelay time.Duration
}

func NewCatalogHTTPClient(baseURL string, timeout time.Duration) *CatalogHTTPClient {
	return &CatalogHTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		maxRetries: 3,
		retryDelay: 100 * time.Millisecond,
	}
}

type productResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Price     float64   `json:"price"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *CatalogHTTPClient) GetByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	url := fmt.Sprintf("%s/products/%s", c.baseURL, id.String())

	var lastErr error

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return domain.Product{}, ctx.Err()
			case <-time.After(c.retryDelay * time.Duration(1<<attempt)): // exponential backoff
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return domain.Product{}, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue // retry on network error
		}

		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return domain.Product{}, ErrProductNotFound
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error from catalog: %d", resp.StatusCode)
			continue // retry on 5xx
		}

		if resp.StatusCode != http.StatusOK {
			return domain.Product{}, fmt.Errorf("unexpected status from catalog: %d", resp.StatusCode)
		}

		var prodResp productResponse
		if err := json.NewDecoder(resp.Body).Decode(&prodResp); err != nil {
			return domain.Product{}, fmt.Errorf("failed to decode catalog response: %w", err)
		}

		return domain.Product{
			ID:        prodResp.ID,
			Name:      prodResp.Name,
			Price:     prodResp.Price,
			CreatedAt: prodResp.CreatedAt,
		}, nil
	}

	return domain.Product{}, fmt.Errorf("%w: %v", ErrServiceUnavailable, lastErr)
}
