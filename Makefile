.PHONY: all build run test clean deps proto docker

# Variables
BINARY_NAME=fanet-api
DOCKER_IMAGE=flybeeper/fanet-api
VERSION=$(shell git describe --tags --always --dirty)
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

# Build the binary
all: build

build:
	@echo "Building..."
	@go build -ldflags="-X 'main.Version=$(VERSION)'" -o $(GOBIN)/$(BINARY_NAME) cmd/fanet-api/main.go

# Run the application
run: build
	@echo "Running..."
	@$(GOBIN)/$(BINARY_NAME)

# Run with hot reload (requires air)
dev:
	@air

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@go install github.com/cosmtrek/air@latest

# Generate protobuf files
proto:
	@echo "Generating protobuf files..."
	@protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		ai-spec/api/fanet.proto
	@mv ai-spec/api/fanet.pb.go pkg/pb/
	@mv ai-spec/api/fanet_grpc.pb.go pkg/pb/

# Run tests
test:
	@echo "Testing..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./...

# Lint the code
lint:
	@echo "Linting..."
	@golangci-lint run

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(GOBIN)
	@rm -f coverage.out coverage.html

# Docker commands
docker-build:
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .

docker-run:
	@docker run -p 8080:8080 --rm $(DOCKER_IMAGE):latest

docker-push:
	@echo "Pushing Docker image..."
	@docker push $(DOCKER_IMAGE):$(VERSION)
	@docker push $(DOCKER_IMAGE):latest

# Development environment
dev-env:
	@docker-compose -f deployments/docker/docker-compose.yml up -d

dev-env-down:
	@docker-compose -f deployments/docker/docker-compose.yml down

# Database migrations
migrate-up:
	@echo "Running migrations..."
	@migrate -path database/migrations -database "mysql://root:password@tcp(localhost:3306)/fanet?parseTime=true" up

migrate-down:
	@migrate -path database/migrations -database "mysql://root:password@tcp(localhost:3306)/fanet?parseTime=true" down

# Generate swagger docs
swagger:
	@echo "Generating swagger docs..."
	@swag init -g cmd/fanet-api/main.go -o docs

# Performance profiling
profile-cpu:
	@go test -cpuprofile=cpu.prof -bench=.
	@go tool pprof -http=:8081 cpu.prof

profile-mem:
	@go test -memprofile=mem.prof -bench=.
	@go tool pprof -http=:8081 mem.prof

# Help
help:
	@echo "Available targets:"
	@echo "  make build    - Build the binary"
	@echo "  make run      - Build and run"
	@echo "  make dev      - Run with hot reload"
	@echo "  make test     - Run tests"
	@echo "  make bench    - Run benchmarks"
	@echo "  make lint     - Lint the code"
	@echo "  make clean    - Clean build artifacts"
	@echo "  make docker-build - Build Docker image"
	@echo "  make dev-env  - Start development environment"
	@echo "  make proto    - Generate protobuf files"