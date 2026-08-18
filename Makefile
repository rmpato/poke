# poke + pogo
#
# `make` builds both binaries into ./bin. `make check` runs everything CI runs,
# so a green local check means a green pull request.

BINDIR   ?= bin
PREFIX   ?= /usr/local
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null)
MODULE   := github.com/rmpato/poke
LDFLAGS  := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT)

GO ?= go

.DEFAULT_GOAL := build
.PHONY: build poke pogo install uninstall test race cover vet fmt fmt-check lint tidy-check check clean run demo help

build: poke pogo ## Build both binaries into ./bin

poke: ## Build the poke CLI
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BINDIR)/poke ./cmd/poke

pogo: ## Build the pogo TUI
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BINDIR)/pogo ./cmd/pogo

install: ## Install both binaries into $$PREFIX/bin (default /usr/local/bin)
	install -d $(DESTDIR)$(PREFIX)/bin
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DESTDIR)$(PREFIX)/bin/poke ./cmd/poke
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DESTDIR)$(PREFIX)/bin/pogo ./cmd/pogo

uninstall: ## Remove installed binaries
	rm -f $(DESTDIR)$(PREFIX)/bin/poke $(DESTDIR)$(PREFIX)/bin/pogo

test: ## Run the test suite
	$(GO) test ./...

race: ## Run the test suite with the race detector
	$(GO) test -race ./...

cover: ## Run tests and open a coverage report
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out

vet: ## Run go vet
	$(GO) vet ./...

fmt: ## Format all Go source
	gofmt -s -w .

fmt-check: ## Fail if any file needs formatting
	@files=$$(gofmt -s -l .); \
	if [ -n "$$files" ]; then echo "not gofmt'd:"; echo "$$files"; exit 1; fi

tidy-check: ## Fail if go.mod/go.sum are not tidy
	@cp go.mod go.mod.bak && cp go.sum go.sum.bak
	@$(GO) mod tidy
	@if ! diff -q go.mod go.mod.bak >/dev/null || ! diff -q go.sum go.sum.bak >/dev/null; then \
		mv go.mod.bak go.mod; mv go.sum.bak go.sum; \
		echo "go.mod/go.sum are not tidy; run: go mod tidy"; exit 1; \
	fi
	@rm -f go.mod.bak go.sum.bak

check: fmt-check vet test race ## Everything CI runs

clean: ## Remove build output
	rm -rf $(BINDIR) coverage.out

run: pogo ## Build and open the TUI
	./$(BINDIR)/pogo

help: ## List targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
