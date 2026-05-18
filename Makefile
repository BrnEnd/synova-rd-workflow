.PHONY: setup build build-lambda zip deploy test test-unit test-integration run docker-up docker-down

VERSION := $(shell cat VERSION)

## setup: download dependencies
setup:
	go mod tidy

## build: compile the binary
build:
	go build -ldflags="-X main.Version=$(VERSION)" -o bin/synova-rd-workflow ./cmd/api

## build-lambda: compile arm64 Lambda bootstrap and zip it for Terraform
build-lambda:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o bootstrap ./cmd/lambda
	powershell -NoProfile -Command "Compress-Archive -Path bootstrap -DestinationPath function.zip -Force"
	powershell -NoProfile -Command "Remove-Item -LiteralPath bootstrap"

## zip: alias for build-lambda
zip: build-lambda

## deploy: build Lambda package and apply Terraform
deploy: build-lambda
	terraform -chdir=terraform init
	terraform -chdir=terraform apply

## test-unit: run all unit tests
test-unit:
	go test ./internal/service/... ./internal/store/... -v -count=1

## test-integration: run integration tests
test-integration:
	go test ./internal/integration/... -v -count=1

## test: run all tests
test: test-unit test-integration

## run: run the API locally (requires .env)
run:
	go run ./cmd/api

## docker-up: start DynamoDB Local + api via docker compose
docker-up:
	docker compose -f docker-compose.dev.yml up --build -d

## docker-down: stop and remove containers
docker-down:
	docker compose -f docker-compose.dev.yml down
