.PHONY: all build run test lint clean seed web-install web-build web-dev dev-backend dev-frontend

# Variables
BINARY_NAME := fyom
BUILD_DIR := ./build
CMD_PATH := ./cmd/fyom
WEB_DIR := ./web
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
    -X main.version=$(VERSION) \
    -X main.gitCommit=$(GIT_COMMIT) \
    -X main.buildTime=$(BUILD_TIME)

all: lint test web-build build

# ==========================================
# Backend Targets
# ==========================================

build:
    @echo ">>> Building backend..."
    @mkdir -p $(BUILD_DIR)
    CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

run: build
    $(BUILD_DIR)/$(BINARY_NAME)

dev-backend:
    CGO_ENABLED=0 go run $(CMD_PATH)

test:
    go test -v -race -coverprofile=coverage.out ./...
    @go tool cover -func=coverage.out | tail -1

test-short:
    go test -v -short ./...

lint:
    @which golangci-lint > /dev/null 2>&1 || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
    golangci-lint run ./...

fmt:
    go fmt ./...
    @which goimports > /dev/null 2>&1 && goimports -w . || true

seed:
	@echo ">>> Seeding: start the server and run these commands:"
	@echo "  curl -X POST http://localhost:8080/api/v1/auth/register \\"
	@echo "    -H 'Content-Type: application/json' \\"
	@echo "    -d '{\"username\":\"admin\",\"password\":\"admin123\"}'"
	@echo "  curl -X POST http://localhost:8080/api/v1/auth/login \\"
	@echo "    -H 'Content-Type: application/json' \\"
	@echo "    -d '{\"username\":\"admin\",\"password\":\"admin123\"}'"

clean:
    rm -rf $(BUILD_DIR) coverage.out
    rm -rf data/
    rm -rf $(WEB_DIR)/dist

# ==========================================
# Frontend Targets (Vue + Vite)
# ==========================================

web-install:
    @echo ">>> Installing frontend dependencies..."
    cd $(WEB_DIR) && npm install

web-dev:
    @echo ">>> Starting frontend dev server..."
    cd $(WEB_DIR) && npm run dev

web-build:
    @echo ">>> Building frontend for production..."
    cd $(WEB_DIR) && npm run build

# ==========================================
# Utilities
# ==========================================

deps:
    go mod download
    go mod tidy

.DEFAULT_GOAL := build
