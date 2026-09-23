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

# THE TWO PASSES, AND WHY THEY ARE TWO.
#
# The race detector costs this project's firmware tests EIGHT TIMES their plain runtime -- measured
# 2026-09-23 on one real-firmware test, 1.88s against 15.06s -- and it is looking for something that
# cannot be there. These tests drive a deliberately single-threaded emulator; "no goroutine in the
# instruction loop" is one of the recorded architecture decisions, and concurrency lives at the
# edges. At eight times, the firmware package alone runs for hours: the race pass was not slow, it
# was UNRUNNABLE inside any timeout worth setting, and a check nobody can run gates nothing. That is
# what this target used to be, and raising the cap three times is what kept it looking fine.
#
# So the boxes SKIP UNDER -race, gated at the two restoredBox fixtures the way they already skip
# under -short, and the detector runs over everything else -- internal/web and the transport, where
# a race can actually live, and where it costs seconds. The two firmware packages complete the race
# pass in about eleven seconds between them.
#
# The plain pass then runs everything INCLUDING the boxes, which is where they are actually
# exercised. Its timeout is a HANG guard and nothing else: every firmware loop runs under an
# instruction budget and fails BY NAME when the machine does not do what it was waiting for (see
# internal/multiplex/firmwaretests/rununtil_test.go), so a genuine hang is reported as "the box
# never asked" rather than as a timeout. Before raising this again, check that guard still holds --
# the two occasions this package crossed ten minutes were both fixed budgets, not slow machines.
#
# CI is unaffected either way: the firmware is not redistributable, so these tests skip there.
test-race: ## Run the race detector where races can be, then the firmware boxes plain
	go test -race -timeout 20m ./...
	go test -timeout 150m ./...

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
