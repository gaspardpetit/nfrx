VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)


OPENAPI=api/openapi.yaml
GENDIR=api/generated

.PHONY: build test lint generate check

LDFLAGS ?= -s -w -X main.version=$(VERSION) -X main.buildSHA=$(BUILD_SHA) -X main.buildDate=$(BUILD_DATE)
GOFLAGS ?= -trimpath

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" ./server/cmd/nfrx
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" ./modules/llm/agent/cmd/nfrx-llm
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" ./modules/mcp/agent/cmd/nfrx-mcp

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

generate:
	rm -rf $(GENDIR) && mkdir -p $(GENDIR)
	oapi-codegen -generate types      -o $(GENDIR)/types.gen.go      -package generated $(OPENAPI)
	oapi-codegen -generate chi-server -o $(GENDIR)/server.gen.go     -package generated $(OPENAPI)
	oapi-codegen -generate spec       -o $(GENDIR)/spec.gen.go       -package generated $(OPENAPI)

check:
	oapi-codegen -generate types -o /dev/null -package tmp $(OPENAPI)
