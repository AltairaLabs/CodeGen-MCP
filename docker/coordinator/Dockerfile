# Multi-stage build for CodeGen-MCP Coordinator
# Lightweight image containing only the coordinator binary

# Stage 1: Build the coordinator binary
FROM golang:1.25-bookworm AS builder

WORKDIR /build

# Copy go.mod and go.sum for dependency caching
COPY go.mod go.sum ./
COPY go.work* ./
RUN go mod download

# Copy source code
COPY api/ ./api/
COPY cmd/coordinator/ ./cmd/coordinator/
COPY internal/ ./internal/

# Build the coordinator binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o coordinator ./cmd/coordinator

# Stage 2: Minimal runtime image
FROM debian:bookworm-slim

# Install minimal runtime dependencies
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       netcat-openbsd \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -g 1000 coordinator \
    && useradd -m -u 1000 -g coordinator -s /bin/bash coordinator

WORKDIR /app

# Copy the coordinator binary from builder stage
COPY --from=builder /build/coordinator /app/coordinator
RUN chmod +x /app/coordinator \
    && chown coordinator:coordinator /app/coordinator

# Switch to non-root user
USER coordinator

# Set environment variables
ENV GRPC_PORT=50051 \
    LOG_LEVEL=info

# Expose gRPC port
EXPOSE 50051

# Run the coordinator
ENTRYPOINT ["/app/coordinator"]
CMD []
