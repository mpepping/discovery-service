.PHONY: help build build-amd64 build-arm64 run test clean docker-build docker-run lint fmt vet

# Variables
BINARY_NAME=discovery-service
BINARY_PATH=bin/$(BINARY_NAME)
DOCKER_IMAGE=discovery-service
DOCKER_TAG=latest
GO=go
GOFLAGS=-v

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: build-amd64 build-arm64 ## Build binaries for amd64 and arm64

build-amd64: ## Build amd64 binary
	@echo "Building $(BINARY_NAME) for amd64..."
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/$(BINARY_NAME)-amd64 ./cmd/discovery-service
	@echo "Binary built: bin/$(BINARY_NAME)-amd64"

build-arm64: ## Build arm64 binary
	@echo "Building $(BINARY_NAME) for arm64..."
	@mkdir -p bin
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/$(BINARY_NAME)-arm64 ./cmd/discovery-service
	@echo "Binary built: bin/$(BINARY_NAME)-arm64"

run: ## Build and run the service (local architecture)
	@echo "Building $(BINARY_NAME) for local use..."
	@mkdir -p bin
	$(GO) build $(GOFLAGS) -o $(BINARY_PATH) ./cmd/discovery-service
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_PATH) -debug

test: ## Run tests
	@echo "Running tests..."
	$(GO) test -v -race -cover ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	$(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint: ## Run linter
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed, run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run

fmt: ## Format code
	@echo "Formatting code..."
	$(GO) fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	$(GO) vet ./...

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

docker-run: docker-build ## Build and run Docker container
	@echo "Running Docker container..."
	docker run --rm -p 3000:3000 -p 3001:3001 -p 2122:2122 \
		$(DOCKER_IMAGE):$(DOCKER_TAG) -debug

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

proto-gen: ## Generate protobuf code (requires protoc and plugins)
	@echo "Generating protobuf code..."
	@which protoc > /dev/null || (echo "protoc not installed" && exit 1)
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/cluster/v1/cluster.proto

check: fmt vet lint test ## Run all checks (fmt, vet, lint, test)

install: build ## Install the binary to GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	cp $(BINARY_PATH) $(GOPATH)/bin/$(BINARY_NAME)

all: clean deps build test ## Clean, download deps, build, and test

.DEFAULT_GOAL := help
