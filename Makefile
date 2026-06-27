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

.PHONY: trust
trust: ## Seed the test-fabric nodes with the backend's CA (run once after `make up`)
	@bash scripts/install-ca.sh

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

.PHONY: enroll-agent
enroll-agent: ## Build the SSH-agent enrollment bridge for this machine's platform
	docker run --rm -v $(PWD)/backend:/src -w /src -e CGO_ENABLED=0 golang:1.23-alpine \
	  sh -c "apk add --no-cache git >/dev/null && GOFLAGS=-mod=mod go build -o /src/bin/fleet-enroll-agent ./cmd/fleet-enroll-agent"
	@echo "Built backend/bin/fleet-enroll-agent — distribute to operators."

.PHONY: enroll-agent-all
enroll-agent-all: ## Cross-compile the bridge for macOS/Linux/Windows (operators' laptops)
	docker run --rm -v $(PWD)/backend:/src -w /src -e CGO_ENABLED=0 golang:1.23-alpine sh -c '\
	  apk add --no-cache git >/dev/null; \
	  set -e; \
	  for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do \
	    os=$${t%/*}; arch=$${t#*/}; ext=; [ "$$os" = windows ] && ext=.exe; \
	    out=/src/bin/fleet-enroll-agent-$$os-$$arch$$ext; \
	    echo "building $$out"; \
	    GOOS=$$os GOARCH=$$arch GOFLAGS=-mod=mod go build -trimpath -ldflags "-s -w" -o $$out ./cmd/fleet-enroll-agent; \
	  done'
	@echo "Built backend/bin/fleet-enroll-agent-* — distribute the right one per operator:"
	@echo "  macOS Apple Silicon: fleet-enroll-agent-darwin-arm64"
	@echo "  macOS Intel:         fleet-enroll-agent-darwin-amd64"
	@echo "  Linux x86_64:        fleet-enroll-agent-linux-amd64"
	@echo "  Linux ARM64:         fleet-enroll-agent-linux-arm64"
	@echo "  Windows x86_64:      fleet-enroll-agent-windows-amd64.exe"

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

.PHONY: e2e
e2e: ## Run Playwright end-to-end tests against the running stack
	-$(COMPOSE_FABRIC) exec -T backend fleetctl create-admin e2euser 'E2e-Pass-12345!' 2>/dev/null
	docker run --rm --network host -v $(PWD)/frontend:/app -w /app \
	  -e E2E_BASE=$${E2E_BASE:-http://localhost:5173} \
	  -e E2E_USER=e2euser -e E2E_PASS='E2e-Pass-12345!' \
	  mcr.microsoft.com/playwright:v1.48.2-jammy \
	  sh -c "npm install >/dev/null 2>&1 && npx playwright test"

.PHONY: load
load: ## Run the k6 load smoke test against the running stack (override USER/PASS)
	docker run --rm --network host -v $(PWD)/deploy/load:/load \
	  -e BASE=$${BASE:-http://localhost:8080} \
	  -e USER=$${FLEET_LOAD_USER:-admin} \
	  -e PASS=$${FLEET_LOAD_PASS:-Sup3r-Secret-Pass!} \
	  grafana/k6 run /load/k6-smoke.js
