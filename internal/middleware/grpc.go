package middleware

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func LoggingUnaryServerInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	start := time.Now()

	resp, err := handler(ctx, req)

	duration := time.Since(start)
	code := status.Code(err)

	if err != nil {
		if code == codes.NotFound || code == codes.InvalidArgument {
			log.Printf("[gRPC Server] %s -> %s (%v) in %v", info.FullMethod, code, err, duration)
		} else {
			log.Printf("[gRPC Server] %s -> ERROR %s (%v) in %v", info.FullMethod, code, err, duration)
		}
	} else {
		log.Printf("[gRPC Server] %s -> %s in %v", info.FullMethod, code, duration)
	}

	return resp, err
}

func RecoveryUnaryServerInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp any, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[gRPC Server] PANIC recovered in %s: %v", info.FullMethod, r)
			err = status.Errorf(codes.Internal, "internal server error: panic recovered")
		}
	}()

	return handler(ctx, req)
}
