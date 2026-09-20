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

# Go's default per-package timeout is ten minutes, and it exists to catch a
# HUNG test. It is not the right guard here: internal/broadcast and
# internal/multiplex run real firmware for hundreds of millions of instructions
# -- a couple of dozen boxes restored from a snapshot -- and the race detector
# costs that about twelve times its plain runtime, so a minute and a quarter
# here is a quarter of an hour there. It is sized for internal/multiplex, which
# is the slowest, and it has needed raising twice as that package grew.
#
# Their own loops are the hang detector now. Every firmware loop runs under a
# budget and fails BY NAME when the machine does not do what it was waiting for
# -- see internal/multiplex/rununtil_test.go -- so a hang is reported as "the
# box never asked" rather than as a timeout, which is a better failure anyway.
# Raising this stops a slow but healthy run being reported as a hang; it does
# not remove a guard, because the guard moved inside. That is only true while
# the guard holds, so BEFORE RAISING IT AGAIN, check that every firmware loop
# still early-exits: the two occasions this package crossed ten minutes were
# both fixed budgets, not slow machines, and a timeout raised over one of those
# is hiding a bug rather than paying for the detector.
#
# CI is unaffected either way: the firmware is not redistributable, so these
# tests skip there.
test-race: ## Run the tests under the race detector
	go test -race -timeout 30m ./...

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
