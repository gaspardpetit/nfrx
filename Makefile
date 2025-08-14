VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: build test lint generate

LDFLAGS ?= -s -w -X main.version=$(VERSION) -X main.buildSHA=$(BUILD_SHA) -X main.buildDate=$(BUILD_DATE)
GOFLAGS ?= -trimpath

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" ./cmd/llamapool-server
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" ./cmd/llamapool-worker

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

generate:
	# reserved for future codegen
