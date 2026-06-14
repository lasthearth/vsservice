# vsservice — common developer tasks. Run `make` or `make help` for the list.
SHELL := /bin/sh

BIN      ?= /bin/vsservice
GOLANGCI := ./custom-gcl

.DEFAULT_GOAL := help

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed -e 's/## //'

# ---------------------------------------------------------------------------
# Application
# ---------------------------------------------------------------------------

## build: build the service binary
build:
	CGO_ENABLED=1 go build -o $(BIN) ./main.go

## run: run the service locally (requires .env)
run:
	go run ./main.go

# ---------------------------------------------------------------------------
# Lint — modelguard plugin, bundled into a custom golangci-lint binary
# ---------------------------------------------------------------------------

## lint: run modelguard over the whole module
lint: $(GOLANGCI)
	$(GOLANGCI) run ./...

## lint-fix: run modelguard with autofix
lint-fix: $(GOLANGCI)
	$(GOLANGCI) run --fix ./...

## lint-build: (re)build the plugin-bundled linter binary
lint-build: $(GOLANGCI)

# custom-gcl is rebuilt whenever the build config or plugin sources change.
$(GOLANGCI): .custom-gcl.yml tools/modelguard/go.mod $(wildcard tools/modelguard/*.go)
	golangci-lint custom

# ---------------------------------------------------------------------------
# Codegen / housekeeping
# ---------------------------------------------------------------------------

## test: run module and plugin tests
test:
	go test ./...
	go -C tools/modelguard test ./...

## generate: regenerate goverter mappers
generate:
	go generate ./...

## proto: regenerate protobuf stubs (requires buf)
proto:
	buf generate

## tidy: tidy both go modules
tidy:
	go mod tidy
	go -C tools/modelguard mod tidy

## hooks: enable the git pre-commit hook (runs `make lint`)
hooks:
	git config core.hooksPath .githooks
	@echo "pre-commit hook enabled (core.hooksPath=.githooks)"

## clean: remove the custom linter binary
clean:
	rm -f $(GOLANGCI)

.PHONY: help build run lint lint-fix lint-build test generate proto tidy hooks clean
