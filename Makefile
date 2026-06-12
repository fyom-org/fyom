# Thin compatibility layer — all real logic lives in Taskfile.yml
# Usage: make test  (equivalent to: task test)

.PHONY: dev dev-web build test test-go test-race coverage lint lint-go lint-web smoke verify ci clean sidecar

dev:
	task dev

dev-web:
	task dev:web

build:
	task build

build-web:
	task build:web

test:
	task test

test-go:
	task test:go

test-race:
	task test:race

coverage:
	task coverage

lint:
	task lint

lint-go:
	task lint:go

lint-web:
	task lint:web

smoke:
	task smoke

verify:
	task verify

ci:
	task ci

sidecar:
	task sidecar

clean:
	task clean
