.PHONY: build test lint generate

build:
	go build ./cmd/llamapool-server
	go build ./cmd/llamapool-worker

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

generate:
	# reserved for future codegen
