.PHONY: build test lint generate

LDFLAGS ?=

build:
	go build -ldflags "$(LDFLAGS)" ./cmd/llamapool-server
	go build -ldflags "$(LDFLAGS)" ./cmd/llamapool-worker

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

generate:
	# reserved for future codegen
