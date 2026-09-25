package grpc

import (
	"context"
	"errors"

	"github.com/Nuriklan/digital-commerce/internal/domain"
	"github.com/Nuriklan/digital-commerce/internal/repository"
	"github.com/Nuriklan/digital-commerce/internal/service"
	catalogpb "github.com/Nuriklan/digital-commerce/proto/catalog"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type CatalogGRPCServer struct {
	catalogpb.CatalogServiceServer
	service *service.ProductService
}

func NewCatalogGRPCServer(svc *service.ProductService) *CatalogGRPCServer {
	return &CatalogGRPCServer{
		service: svc,
	}
}

func (s *CatalogGRPCServer) GetProduct(ctx context.Context, req *catalogpb.GetProductRequest) (*catalogpb.GetProductResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "product id is required")
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid product id format")
	}

	product, err := s.service.GetProduct(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "product not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get product: %v", err)
	}

	return &catalogpb.GetProductResponse{
		Product: toProtoProduct(product),
	}, nil
}

func (s *CatalogGRPCServer) ListProducts(ctx context.Context, req *catalogpb.ListProductsRequest) (*catalogpb.ListProductsResponse, error) {
	products, err := s.service.ListProducts(ctx, 100, 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list products: %v", err)
	}

	protoProducts := make([]*catalogpb.Product, 0, len(products))
	for _, p := range products {
		protoProducts = append(protoProducts, toProtoProduct(p))
	}

	return &catalogpb.ListProductsResponse{
		Products: protoProducts,
	}, nil
}

func toProtoProduct(p domain.Product) *catalogpb.Product {
	return &catalogpb.Product{
		Id:        p.ID.String(),
		Name:      p.Name,
		Price:     p.Price,
		CreatedAt: timestamppb.New(p.CreatedAt),
	}
}
