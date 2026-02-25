.PHONY: build run test docker-build docker-up prepare-dirs preflight smoke prod-up prod-down logs db-psql migrate up down

BIN_DIR ?= bin
BINARY ?= $(BIN_DIR)/turcompany
ROOT_DIR ?= files
COMPOSE_PROD ?= docker compose -f docker-compose.prod.yml

build: prepare-dirs
	mkdir -p $(BIN_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BINARY) ./cmd/web

run: prepare-dirs
	go run ./cmd/web

test:
	go test ./...

prepare-dirs:
	mkdir -p $(ROOT_DIR) $(ROOT_DIR)/pdf $(ROOT_DIR)/docx $(ROOT_DIR)/excel

docker-build:
	docker build -t turcompany-backend .

docker-up: up

up:
	$(COMPOSE_PROD) up -d --build

down:
	$(COMPOSE_PROD) down --remove-orphans

preflight:
	./scripts/preflight.sh

smoke:
	SMOKE_ONLY=1 ./scripts/preflight.sh

prod-up: up

prod-down: down

logs:
	$(COMPOSE_PROD) logs -f --tail=200

db-psql:
	$(COMPOSE_PROD) exec postgres psql -U $${POSTGRES_USER:-turcompany} -d $${POSTGRES_DB:-turcompany}

migrate:
	$(COMPOSE_PROD) run --rm migrate

# backward-compatible aliases
up-prod: up

down-prod: down

psql: db-psql
