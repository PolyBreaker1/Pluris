## Pluris dev Makefile (Increment 1)
##
## Common targets:
##   make tools       - install Go dev tools (templ)
##   make gen         - run templ codegen
##   make dev         - run console at http://localhost:8080
##   make build       - build console binary into bin/
##   make test        - run all tests (incl. mount-point tests)
##   make doctor      - check tooling
##   make clean       - remove generated code + binaries

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

GO        ?= go
GOBIN     := $(shell $(GO) env GOPATH 2>/dev/null)/bin
TEMPL     := $(GOBIN)/templ

## help: show available targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

## doctor: verify required tools
.PHONY: doctor
doctor:
	@command -v $(GO) >/dev/null && echo "go:    $$($(GO) version)" || (echo "go:    MISSING (apt install golang-go)"; exit 1)
	@[ -x "$(TEMPL)" ] && echo "templ: installed at $(TEMPL)" || echo "templ: MISSING (make tools)"

## tools: install Go-based dev tools
.PHONY: tools
tools:
	$(GO) install github.com/a-h/templ/cmd/templ@v0.2.793

## gen: regenerate templ files
.PHONY: gen
gen:
	$(TEMPL) generate ./web/templates

## dev: run console at :8080 (regenerates templ first)
.PHONY: dev
dev: gen
	$(GO) run ./cmd/console

## build: build console binary
.PHONY: build
build: gen
	mkdir -p bin
	$(GO) build -o bin/pluris-console ./cmd/console

## test: run tests (templ generation prerequisite)
.PHONY: test
test: gen
	$(GO) test ./...

## vet: go vet
.PHONY: vet
vet: gen
	$(GO) vet ./...

## clean: remove generated code + binaries
.PHONY: clean
clean:
	rm -rf bin
	find web/templates -name '*_templ.go' -delete
