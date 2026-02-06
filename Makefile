.PHONY: build run test docker-build docker-up prepare-dirs up-prod down-prod logs migrate up-dev down-dev logs-dev psql

BIN_DIR ?= bin
BINARY ?= $(BIN_DIR)/turcompany
ROOT_DIR ?= files
COMPOSE_PROD ?= docker compose -f docker-compose.prod.yml
COMPOSE_DEV ?= docker compose -f docker-compose.dev.yml

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

docker-up:
	docker-compose up -d

up-prod:
	$(COMPOSE_PROD) --profile db up -d --build

down-prod:
	$(COMPOSE_PROD) down

logs:
	$(COMPOSE_PROD) logs -f --tail=200

migrate:
	$(COMPOSE_PROD) run --rm migrate

up-dev:
	$(COMPOSE_DEV) up -d --build

down-dev:
	$(COMPOSE_DEV) down

logs-dev:
	$(COMPOSE_DEV) logs -f --tail=200

psql:
	$(COMPOSE_DEV) exec postgres psql -U $${POSTGRES_USER:-turcompany} -d $${POSTGRES_DB:-turcompany}
