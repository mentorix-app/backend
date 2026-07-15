# Mentorix backend — daily targets. Heavy logic lives in scripts/.
.DEFAULT_GOAL := help

export PATH := $(shell go env GOPATH)/bin:$(PATH)

ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
SCRIPTS := $(ROOT)scripts
GOPATH_BIN := $(shell go env GOPATH)/bin
AIR := $(GOPATH_BIN)/air

MIGRATE_VERSION ?= v4.18.2
SQLC_VERSION ?= v1.29.0
LINT_VERSION ?= v1.62.2
AIR_VERSION ?= v1.61.7

.PHONY: help dev run build setup \
	migrate migrate-down schema-sync sqlc \
	check check-ci \
	install-hooks install-tools \
	install-migrate install-sqlc install-lint install-air

help: ## Show commands
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\n"} \
		/^[a-zA-Z0-9_.-]+:.*##/ { printf "  %-14s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# --- run ---

dev: ## API with hot reload (Air)
	@test -x '$(AIR)' || $(MAKE) --no-print-directory install-air
	@$(AIR)

run: ## API once (go run)
	go run ./cmd/api

build: ## Build bin/api
	@mkdir -p bin
	go build -o bin/api ./cmd/api

setup: install-hooks ## First time: .env, migrate, pre-commit
	@test -f .env || (cp .env.example .env && echo "Created .env from .env.example")
	@$(MAKE) migrate

# --- db ---

migrate: ## Apply migrations (up)
	$(SCRIPTS)/migrate.sh up

migrate-down: ## Roll back one migration (N=2 for two, …)
	$(SCRIPTS)/migrate.sh down $(or $(N),1)

schema-sync: ## Regenerate db/schema.sql from migrations
	$(SCRIPTS)/schema-sync.sh

sqlc: ## sqlc generate
	@command -v sqlc >/dev/null 2>&1 || $(MAKE) --no-print-directory install-sqlc
	sqlc generate

# --- qa ---

check: ## Full local QA (migrate-check + smoke if API up)
	$(SCRIPTS)/check.sh $(CHECK_FLAGS)

check-ci: ## Same as GitHub CI / pre-commit
	$(SCRIPTS)/check.sh --ci $(CHECK_FLAGS)

# --- tooling ---

install-hooks: ## Install pre-commit → make check-ci
	@test -d .git/hooks || (echo "Not a git repository (.git/hooks missing)" >&2 && exit 1)
	@cp $(SCRIPTS)/git-hooks/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Installed .git/hooks/pre-commit → make check-ci"

install-tools: install-migrate install-sqlc install-lint install-air ## Install migrate, sqlc, lint, air

install-migrate:
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

install-sqlc:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)

install-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(LINT_VERSION)

install-air:
	go install github.com/air-verse/air@$(AIR_VERSION)
