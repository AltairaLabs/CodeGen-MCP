# Multi-stage builder image for CodeGen-MCP Worker
# - Purpose: provide a reproducible environment for running the worker server
#   with Python project building, testing, and execution capabilities.
# - Includes: Python 3.11, Go 1.25, worker binary, build tools

# Stage 1: Build the worker binary
FROM golang:1.25-bookworm AS go-builder

WORKDIR /build

# Copy go.mod and go.sum for dependency caching
COPY go.mod go.sum ./
COPY go.work* ./
RUN go mod download

# Copy source code
COPY api/ ./api/
COPY cmd/worker/ ./cmd/worker/
COPY internal/ ./internal/

# Build the worker binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o worker ./cmd/worker

# Stage 2: Runtime image with Python and worker binary
FROM python:3.11-slim AS base

# Metadata for reproducible builds
ARG BUILD_DATE=""
ARG VCS_REF=""
LABEL org.opencontainers.image.created=$BUILD_DATE \
      org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.title="codegen-mcp-python-builder" \
      org.opencontainers.image.description="Builder image for CodeGen-MCP POC"

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=off \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_DEFAULT_TIMEOUT=60

# Install OS-level build deps and useful tools
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       build-essential \
       git \
       curl \
       ca-certificates \
       libssl-dev \
       libffi-dev \
       python3-venv \
       jq \
       gnupg \
       wget \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root user for builds
ARG BUILDER_UID=1000
ARG BUILDER_GID=1000
RUN groupadd -g ${BUILDER_GID} builder \
    && useradd -m -u ${BUILDER_UID} -g builder -s /bin/bash builder

WORKDIR /home/builder/workspace

# Install Python tooling (pip, pip-tools, tox, pytest, mypy, flake8)
RUN python -m pip install --upgrade pip setuptools wheel
RUN pip install \
    pip-tools \
    tox \
    pytest \
    pytest-cov \
    mypy \
    flake8 \
    build \
    twine

# Copy the worker binary from the go-builder stage
COPY --from=go-builder /build/worker /usr/local/bin/worker
RUN chmod +x /usr/local/bin/worker

# Create a directory for pip cache that can be mounted from the host
ENV PIP_CACHE_DIR=/home/builder/.cache/pip
RUN mkdir -p ${PIP_CACHE_DIR} \
    && chown -R builder:builder /home/builder

# Create workspaces directory for worker sessions
RUN mkdir -p /home/builder/workspaces \
    && chown -R builder:builder /home/builder/workspaces

# Switch to non-root
USER builder

# Set environment variables for worker
ENV WORKER_ID=worker-1 \
    GRPC_PORT=50052 \
    MAX_SESSIONS=5 \
    BASE_WORKSPACE=/home/builder/workspaces \
    COORDINATOR_ADDR=coordinator:50051

# Expose gRPC port
EXPOSE 50052

# Default: run the worker server
# Can be overridden for development/testing
ENTRYPOINT ["/usr/local/bin/worker"]
CMD []
