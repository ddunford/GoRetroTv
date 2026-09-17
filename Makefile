SHELL   := /bin/bash
SERVICE := goretrotv
MODULE  := github.com/ddunford/goretrotv
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
  -X $(MODULE)/internal/version.Version=$(VERSION) \
  -X $(MODULE)/internal/version.Commit=$(COMMIT) \
  -X $(MODULE)/internal/version.Date=$(DATE)

.PHONY: build run test test-race test-cover lint vet fmt tidy vuln docker clean help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "}; {printf "  %-12s %s\n", $$1, $$2}'

# Where `build` puts binaries. Overridable so the boot gate can build into a scratch directory
# without disturbing another agent's bin/, and -- more importantly -- so it builds through THIS
# recipe rather than re-implementing it. Two definitions of the build identity is the duplication
# this project keeps paying for: the gate's own "does /health name the build" stage was satisfied
# by construction for exactly that reason.
BIN_DIR ?= bin

build: ## Build every binary into $(BIN_DIR)
	@mkdir -p "$(BIN_DIR)"
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o "$(BIN_DIR)/" ./cmd/...

run: build ## Build and run the emulator server
	./bin/$(SERVICE)

test: ## Run the tests
	go test ./...

test-race: ## Run the tests under the race detector
	go test -race ./...

test-cover: ## Run the tests with coverage
	go test -cover ./...

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint
	golangci-lint run ./cmd/... ./internal/... ./conformance/...

fmt: ## Format the tree
	gofmt -w ./cmd ./internal

tidy: ## Tidy the module
	go mod tidy

vuln: ## Check the stdlib against the Go vulnerability database for anything NEW
	./tools/vulncheck.sh

docker: ## Build the container image
	docker build \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg COMMIT=$(COMMIT) \
	  --build-arg BUILD_DATE=$(DATE) \
	  -t $(SERVICE):$(VERSION) .

clean: ## Remove build artefacts
	rm -rf bin/
