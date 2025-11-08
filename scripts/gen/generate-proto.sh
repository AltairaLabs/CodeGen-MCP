#!/usr/bin/env bash

set -euo pipefail

# Script to generate Go code from protobuf definitions

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
PROTO_DIR="${PROJECT_ROOT}/api/proto/v1"
OUT_DIR="${PROJECT_ROOT}/api/proto/v1"

echo "Generating Go code from protobuf definitions..."
echo "Proto directory: ${PROTO_DIR}"
echo "Output directory: ${OUT_DIR}"

# Check for protoc
if ! command -v protoc &> /dev/null; then
    echo "Error: protoc not found. Please install Protocol Buffers compiler."
    echo "  macOS: brew install protobuf"
    echo "  Linux: apt-get install -y protobuf-compiler"
    exit 1
fi

# Check for protoc-gen-go
if ! command -v protoc-gen-go &> /dev/null; then
    echo "protoc-gen-go not found, installing..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

# Check for protoc-gen-go-grpc
if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "protoc-gen-go-grpc not found, installing..."
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
fi

# Ensure output directory exists
mkdir -p "${OUT_DIR}"

# Generate Go code for all proto files
for proto_file in "${PROTO_DIR}"/*.proto; do
    if [ -f "${proto_file}" ]; then
        echo "Generating from: $(basename "${proto_file}")"
        protoc \
            --proto_path="${PROJECT_ROOT}" \
            --go_out="${PROJECT_ROOT}" \
            --go_opt=paths=source_relative \
            --go-grpc_out="${PROJECT_ROOT}" \
            --go-grpc_opt=paths=source_relative \
            "${proto_file}"
    fi
done

echo "✓ Proto generation complete!"
echo "Generated files in: ${OUT_DIR}"
