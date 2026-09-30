# ==========================================
# Stage 1: Build stage
# ==========================================
FROM golang:alpine AS builder

# Installing certificates and time zones for transfer to the final image
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Caching optimization: download dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire project source code
COPY . .

# The name of the microservice being built is passed via --build-arg SERVICE_NAME=...
ARG SERVICE_NAME
RUN test -n "$SERVICE_NAME" || (echo "SERVICE_NAME build argument is required" && exit 1)

# Building a static binary without CGO, stripping debug information
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /app/bin/service \
    ./cmd/${SERVICE_NAME}

# ==========================================
# Stage 2: Final minimal runtime image
# ==========================================
FROM alpine:3.20 AS runner

# Installation of CA certificates (for HTTPS calls) and tzdata (for time zone support in the `time` package)
RUN apk --no-cache add ca-certificates tzdata

# Security: launching the application as an unprivileged user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup -u 10001

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/bin/service /app/service

# Assigning a file owner
RUN chown -R appuser:appgroup /app

# Switch to a non-root user
USER 10001

# Entry point for launching the microservice
ENTRYPOINT ["/app/service"]