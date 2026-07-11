# Mentorix backend — common tasks. Complex logic stays in scripts/.
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

.PHONY: help dev run build setup docker-up docker-down \
	migrate migrate-down migrate-version migrate-check \
	schema-sync sqlc generate \
	test test-integration vet fmt lint \
	validate validate-smoke check check-ci docs-check \
	coverage coverage-check install-hooks \
	install-tools install-migrate install-sqlc install-lint install-air

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} \
		/^[a-zA-Z0-9_.-]+:.*##/ { printf "  %-18s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

dev: ## Run API with hot reload (Air)
	@test -x '$(AIR)' || $(MAKE) --no-print-directory install-air
	@$(AIR)

run: ## Run API once (go run)
	go run ./cmd/api

build: ## Build API binary to bin/api
	@mkdir -p bin
	go build -o bin/api ./cmd/api

setup: docker-up install-hooks ## First-time local setup: compose, .env, migrations, pre-commit hook
	@test -f .env || (cp .env.example .env && echo "Created .env from .env.example")
	@$(MAKE) migrate

docker-up: ## Start Postgres + Redis (docker compose up -d)
	docker compose up -d

docker-down: ## Stop docker compose services
	docker compose down

migrate: ## Apply database migrations (up)
	$(SCRIPTS)/migrate.sh up

migrate-down: ## Roll back migrations (N=1 by default, e.g. make migrate-down N=2)
	$(SCRIPTS)/migrate.sh down $(or $(N),1)

migrate-version: ## Show current database migration version
	$(SCRIPTS)/migrate.sh version

migrate-check: ## Verify DB matches latest migration in repo
	$(SCRIPTS)/migrate-check.sh

schema-sync: ## Regenerate db/schema.sql from migrations
	$(SCRIPTS)/schema-sync.sh

sqlc: ## Run sqlc generate
	@command -v sqlc >/dev/null 2>&1 || $(MAKE) --no-print-directory install-sqlc
	sqlc generate

generate: ## Run go generate ./internal/db/...
	go generate ./internal/db/...

test: ## Run unit tests (no integration tag)
	go test ./... -count=1

test-integration: ## Run store integration tests (Docker / Testcontainers)
	go test -tags integration -timeout 5m ./internal/db/storetest/... -count=1

vet: ## Run go vet
	go vet ./...

fmt: ## Check gofmt (lists files that need formatting)
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

lint: ## Run golangci-lint
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(LINT_VERSION) run ./...; \
	fi

validate: ## Contract validation (Postman + apicheck, no live API smoke)
	SKIP_SMOKE=1 $(ROOT)postman/validate.sh

validate-smoke: ## Contract validation + smoke on http://localhost:8080
	$(ROOT)postman/validate.sh

check: ## Full local QA suite (see scripts/check.sh --help for flags)
	$(SCRIPTS)/check.sh $(CHECK_FLAGS)

check-ci: ## CI parity checks only
	$(SCRIPTS)/check.sh --ci

docs-check: ## Validate docs/rules links, sizes, migration version in docs
	$(SCRIPTS)/docs-check.sh

install-hooks: ## Install git pre-commit hook (runs make check)
	@test -d .git/hooks || (echo "Not a git repository (.git/hooks missing)" >&2 && exit 1)
	@cp $(SCRIPTS)/git-hooks/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Installed .git/hooks/pre-commit → make check"

coverage: ## Coverage report (unit + integration merge)
	$(SCRIPTS)/coverage.sh

coverage-check: ## Fail if merged coverage is below 85%
	$(SCRIPTS)/coverage.sh --min 85

install-tools: install-migrate install-sqlc install-lint install-air ## Install dev CLI tools via go install

install-migrate:
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

install-sqlc:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)

install-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(LINT_VERSION)

install-air:
	go install github.com/air-verse/air@$(AIR_VERSION)
