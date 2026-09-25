package client

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	catalogpb "github.com/Nuriklan/digital-commerce/proto/catalog"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// loggingClientInterceptor логирует исходящие RPC запросы со стороны клиента
func loggingClientInterceptor(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	start := time.Now()
	err := invoker(ctx, method, req, reply, cc, opts...)
	log.Printf("[gRPC Client] invoked %s in %v, status: %s", method, time.Since(start), status.Code(err))
	return err
}

// CatalogGRPCClient предоставляет доступ к Catalog Service по протоколу gRPC
type CatalogGRPCClient struct {
	conn    *grpc.ClientConn
	client  catalogpb.CatalogServiceClient
	timeout time.Duration
}

// NewCatalogGRPCClient создает новое постоянное соединение с gRPC сервером каталога
func NewCatalogGRPCClient(addr string, timeout time.Duration) (*CatalogGRPCClient, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(loggingClientInterceptor),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc client for %s: %w", addr, err)
	}

	return &CatalogGRPCClient{
		conn:    conn,
		client:  catalogpb.NewCatalogServiceClient(conn),
		timeout: timeout,
	}, nil
}

// Close закрывает сетевое соединение с gRPC сервером
func (c *CatalogGRPCClient) Close() error {
	return c.conn.Close()
}

// GetByID запрашивает товар по ID по gRPC и маппит результат в доменную сущность domain.Product
func (c *CatalogGRPCClient) GetByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.GetProduct(callCtx, &catalogpb.GetProductRequest{
		Id: id.String(),
	})
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.NotFound:
				return domain.Product{}, ErrProductNotFound
			case codes.Unavailable, codes.DeadlineExceeded:
				return domain.Product{}, fmt.Errorf("%w: %v", ErrServiceUnavailable, st.Message())
			}
		}
		return domain.Product{}, fmt.Errorf("catalog grpc error: %w", err)
	}

	prod := resp.GetProduct()
	if prod == nil {
		return domain.Product{}, ErrProductNotFound
	}

	prodID, err := uuid.Parse(prod.GetId())
	if err != nil {
		return domain.Product{}, fmt.Errorf("invalid product id received from catalog: %w", err)
	}

	var createdAt time.Time
	if prod.GetCreatedAt() != nil {
		createdAt = prod.GetCreatedAt().AsTime()
	}

	return domain.Product{
		ID:        prodID,
		Name:      prod.GetName(),
		Price:     prod.GetPrice(),
		CreatedAt: createdAt,
	}, nil
}
