.PHONY: build run dev clean test docker-build docker-up docker-down

# Build the binary
build:
	CGO_ENABLED=1 go build -o bin/slimdeploy ./cmd/slimdeploy

# Run the application
run: build
	./bin/slimdeploy

# Development mode (with hot reload using air if installed)
dev:
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "air not installed, running normally..."; \
		go run ./cmd/slimdeploy; \
	fi

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf data/
	rm -rf deployments/

# Run tests
test:
	go test -v ./...

# Build Docker image
docker-build:
	docker build -t slimdeploy:latest .

# Start with docker-compose
docker-up:
	docker compose up -d

# Stop docker-compose
docker-down:
	docker compose down

# View logs
docker-logs:
	docker compose logs -f slimdeploy

# Rebuild and restart
docker-restart: docker-build
	docker compose up -d --force-recreate slimdeploy

# Development with local Traefik (for testing)
dev-compose:
	docker compose -f docker-compose.dev.yml up -d

# Install dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Lint code (requires golangci-lint)
lint:
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed"; \
	fi

# Generate (if needed in the future)
generate:
	go generate ./...

# Help
help:
	@echo "SlimDeploy Makefile commands:"
	@echo ""
	@echo "  make build        - Build the binary"
	@echo "  make run          - Build and run the application"
	@echo "  make dev          - Run in development mode"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make test         - Run tests"
	@echo "  make docker-build - Build Docker image"
	@echo "  make docker-up    - Start with docker-compose"
	@echo "  make docker-down  - Stop docker-compose"
	@echo "  make docker-logs  - View logs"
	@echo "  make deps         - Install dependencies"
	@echo "  make fmt          - Format code"
	@echo "  make lint         - Lint code"
