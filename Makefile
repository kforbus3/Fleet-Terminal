# Fleet Terminal — developer entrypoints.
# Everything runs through Docker so no local Go/Postgres toolchain is required.

COMPOSE        := docker compose -f deploy/compose/docker-compose.yml
COMPOSE_FABRIC := $(COMPOSE) -f deploy/compose/docker-compose.testfabric.yml

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
	  awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: env
env: ## Create .env from .env.example if missing
	@test -f .env || (cp .env.example .env && echo "created .env")

.PHONY: up
up: env ## Build & start the full stack + test fabric
	$(COMPOSE_FABRIC) up -d --build

.PHONY: up-app
up-app: env ## Start only the application stack (no test fabric)
	$(COMPOSE) up -d --build

.PHONY: down
down: ## Stop the stack
	$(COMPOSE_FABRIC) down

.PHONY: clean
clean: ## Stop the stack and remove volumes (DESTROYS DATA)
	$(COMPOSE_FABRIC) down -v

.PHONY: logs
logs: ## Tail logs
	$(COMPOSE_FABRIC) logs -f --tail=100

.PHONY: ps
ps: ## Show running services
	$(COMPOSE_FABRIC) ps

.PHONY: build
build: ## Build all images
	$(COMPOSE_FABRIC) build

.PHONY: backend-build
backend-build: ## Compile the backend in a throwaway Go container
	docker run --rm -v $(PWD)/backend:/src -w /src golang:1.23-alpine \
	  sh -c "apk add --no-cache git >/dev/null && GOFLAGS=-mod=mod go build ./..."

.PHONY: test
test: backend-test frontend-test ## Run all tests

.PHONY: backend-test
backend-test: ## Run Go unit + integration tests
	docker run --rm -v $(PWD)/backend:/src -w /src golang:1.23-alpine \
	  sh -c "apk add --no-cache git gcc musl-dev openssh-client >/dev/null && GOFLAGS=-mod=mod go test ./..."

.PHONY: frontend-test
frontend-test: ## Run frontend unit tests
	docker run --rm -v $(PWD)/frontend:/app -w /app node:22-alpine \
	  sh -c "npm ci && npm run test -- --run"

.PHONY: lint
lint: ## Run Go vet
	docker run --rm -v $(PWD)/backend:/src -w /src golang:1.23-alpine \
	  sh -c "apk add --no-cache git >/dev/null && GOFLAGS=-mod=mod go vet ./..."

.PHONY: tidy
tidy: ## Run go mod tidy and write go.sum back to the repo
	docker run --rm -v $(PWD)/backend:/src -w /src golang:1.23-alpine \
	  sh -c "apk add --no-cache git >/dev/null && go mod tidy"
