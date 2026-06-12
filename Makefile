# Thin compatibility layer — all real logic lives in justfile
# Usage: make test  (equivalent to: just test)
#
# just is the PRIMARY entry point — shorter commands, better UX.
# This Makefile is a convenience alias for make users.
# Taskfile.yml is also kept in sync for `task` users and CI portability.

.PHONY: dev dev-web build build-web test test-go test-race coverage lint lint-go lint-web smoke verify ci sidecar clean

dev:
	just dev

dev-web:
	just dev-web

build:
	just build

build-web:
	just build-web

test:
	just test

test-go:
	just test-go

test-race:
	just test-race

coverage:
	just coverage

lint:
	just lint

lint-go:
	just lint-go

lint-web:
	just lint-web

smoke:
	just smoke

verify:
	just verify

ci:
	just ci

sidecar:
	just sidecar

clean:
	just clean
