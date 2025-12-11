.PHONY: build run test docker-build docker-up prepare-dirs

BIN_DIR ?= bin
BINARY ?= $(BIN_DIR)/turcompany
ROOT_DIR ?= files

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
