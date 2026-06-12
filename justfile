# justfile — single entry point for all project tasks
# Usage: just <task>  (e.g. just test, just build, just ci)
#
# just vs Taskfile:
#   - just is the PRIMARY entry point (shorter commands, better UX)
#   - Taskfile.yml is kept as an alias for `task` users and CI portability
#   - Both stay in sync — justfile is the canonical source

set shell := ["bash", "-cu"]

# ================================================
# Variables
# ================================================

BINARY_NAME := "fyom"
BUILD_DIR := "./build"
CMD_PATH := "./cmd/fyom"

# ================================================
# Development
# ================================================

# Run Go backend with Air hot-reload (backend pane)
dev:
    air -c .air.toml

# Run Vite dev server (frontend pane)
dev-web:
    cd web && npm run dev -- --host

# ================================================
# Preview
# ================================================

# Run Go backend preview
preview:
    CGO_ENABLED=0 FYOM_SERVER_HOST=127.0.0.1 go run cmd/fyom/main.go -c

# Run Vite preview server
preview-web:
    cd web && npm run preview

# ================================================
# Build
# ================================================

# Build frontend for production
build-web:
    cd web && npm run build:full
    find web/dist -name "*.map" -delete

# Build single binary (embedded frontend)
build: build-web
    mkdir -p {{BUILD_DIR}}
    CGO_ENABLED=0 go build -ldflags "-s -w" -o {{BUILD_DIR}}/{{BINARY_NAME}} {{CMD_PATH}}

# Build sidecar binary (for Tauri sidecar mode)
sidecar:
    mkdir -p {{BUILD_DIR}}
    CGO_ENABLED=0 go build -ldflags "-s -w" -o {{BUILD_DIR}}/{{BINARY_NAME}} {{CMD_PATH}}

# ================================================
# Test
# ================================================

# Run all tests
test: test-go

# Run all Go tests
test-go:
    CGO_ENABLED=0 go test -v -count=1 ./...

# Run Go tests with race detector
test-race:
    CGO_ENABLED=1 go test -race -count=1 ./...

# Run Go coverage report
coverage:
    CGO_ENABLED=0 go test -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out
    go tool cover -html=coverage.out -o coverage.html

# Run production smoke test (build + curl)
smoke:
    bash scripts/smoke_test.sh

# Verify build artifacts (no .map, .gz coverage, refs)
verify:
    bash scripts/verify_bundle.sh

# ================================================
# Lint
# ================================================

# Run all linters
lint: lint-go

# Run golangci-lint
lint-go:
    CGO_ENABLED=0 golangci-lint run ./...

# Run frontend lint
lint-web:
    cd web && npm run lint

# ================================================
# CI
# ================================================

# Run full CI verification suite
ci: lint test build verify

# ================================================
# Cleanup
# ================================================

# Remove build artifacts and temp files
clean:
    rm -rf {{BUILD_DIR}} web/dist tmp/ data/ coverage.out coverage.html
