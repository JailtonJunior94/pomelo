.PHONY: build build-server build-simulator run run-simulator test test-coverage lint fmt vet tidy docker-up docker-down docker-build docker-logs clean help

BINARY_SERVER   := bin/server
BINARY_SIMULATOR := bin/simulator
MODULE          := github.com/jailtonjunior/pomelo
DOCKER_COMPOSE  := docker compose -f deployment/docker-compose.yml

# ── Build ─────────────────────────────────────────────────────────────────────

build: build-server build-simulator

build-server:
	go build -o $(BINARY_SERVER) ./cmd/server

build-simulator:
	go build -o $(BINARY_SIMULATOR) ./cmd/simulator

# ── Run ───────────────────────────────────────────────────────────────────────

run:
	go run ./cmd/server

run-simulator:
	go run ./cmd/simulator

# ── Test ──────────────────────────────────────────────────────────────────────

test:
	go test ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-verbose:
	go test -v ./...

# ── Code quality ──────────────────────────────────────────────────────────────

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

# ── Docker ────────────────────────────────────────────────────────────────────

docker-build:
	$(DOCKER_COMPOSE) build

docker-up:
	$(DOCKER_COMPOSE) up -d

docker-down:
	$(DOCKER_COMPOSE) down

docker-logs:
	$(DOCKER_COMPOSE) logs -f

docker-simulator:
	$(DOCKER_COMPOSE) run --rm simulator

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/ coverage.out coverage.html

# ── Help ──────────────────────────────────────────────────────────────────────

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Build:"
	@echo "  build             Build server and simulator binaries"
	@echo "  build-server      Build server binary"
	@echo "  build-simulator   Build simulator binary"
	@echo ""
	@echo "Run:"
	@echo "  run               Run server locally"
	@echo "  run-simulator     Run simulator locally"
	@echo ""
	@echo "Test:"
	@echo "  test              Run all tests"
	@echo "  test-coverage     Run tests and generate HTML coverage report"
	@echo "  test-verbose      Run tests with verbose output"
	@echo ""
	@echo "Code quality:"
	@echo "  fmt               Format code with gofmt"
	@echo "  vet               Run go vet"
	@echo "  lint              Run golangci-lint"
	@echo "  tidy              Tidy go.mod and go.sum"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build      Build Docker images"
	@echo "  docker-up         Start services in background"
	@echo "  docker-down       Stop and remove containers"
	@echo "  docker-logs       Follow container logs"
	@echo "  docker-simulator  Run simulator container interactively"
	@echo ""
	@echo "  clean             Remove build artifacts"
