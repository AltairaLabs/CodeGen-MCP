# AltairaLabs CodeGen MCP Makefile

.PHONY: all build-coordinator build-worker run-coordinator run-worker clean

all: build-coordinator build-worker

build-coordinator:
	go build -o bin/coordinator ./cmd/coordinator

build-worker:
	go build -o bin/worker ./cmd/worker

run-coordinator:
	go run ./cmd/coordinator

run-worker:
	go run ./cmd/worker

clean:
	rm -rf bin/
