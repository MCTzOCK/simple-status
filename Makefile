BINARY := simple-status
CONFIG ?= simple-status.yml
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build run test cover fmt vet lint tidy docker validate clean

all: build

build: ## Build a static binary into bin/
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/$(BINARY)

run: build ## Build and run with $(CONFIG)
	./bin/$(BINARY) --config $(CONFIG)

validate: build ## Validate $(CONFIG) and exit
	./bin/$(BINARY) --config $(CONFIG) --validate

once: build ## Check every service once and exit
	./bin/$(BINARY) --config $(CONFIG) --once

test: ## Run all tests with the race detector
	go test -race ./...

cover: ## Run tests with coverage and print the total
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

fmt: ## Format all Go sources
	gofmt -l -w .

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint (install: https://golangci-lint.run)
	golangci-lint run

tidy: ## Tidy go.mod / go.sum
	go mod tidy

docker: ## Build the Docker image
	docker build -t $(BINARY):dev .

clean: ## Remove build artifacts
	rm -rf bin coverage.out

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
