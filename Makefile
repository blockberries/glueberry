.PHONY: all build test test-race test-short lint clean coverage fmt vet tidy help
.PHONY: bench bench-crypto bench-all bench-compare benchmark
.PHONY: fuzz fuzz-crypto fuzz-addressbook fuzz-cramberry fuzz-handshake fuzz-list
.PHONY: deps verify check ci examples example-simple-chat

# Go parameters
GO := go
GOFLAGS := -v
TESTFLAGS := -race -coverprofile=coverage.out -covermode=atomic
BENCHFLAGS := -bench=. -benchmem

# Package paths
PKG := ./...

# Version info (can be overridden)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Fuzz testing time (default 30 seconds; override with FUZZTIME=60s make fuzz)
FUZZTIME ?= 30s

# Default target
all: fmt vet lint test build

## Build targets

build: ## Build all packages
	$(GO) build $(GOFLAGS) $(PKG)

## Test targets

test: ## Run tests with race detection and coverage
	$(GO) test $(TESTFLAGS) $(PKG)

test-race: ## Run tests with race detection (alias for test)
	$(GO) test -race -v $(PKG)

test-short: ## Run tests without race detection (faster)
	$(GO) test -short -v $(PKG)

## Benchmark targets

benchmark: bench ## Alias for bench

bench: ## Run benchmarks
	$(GO) test $(BENCHFLAGS) ./benchmark/...
	$(GO) test $(BENCHFLAGS) ./pkg/crypto/...

bench-crypto: ## Run crypto benchmarks only
	$(GO) test $(BENCHFLAGS) ./benchmark/... -run='^$$'
	$(GO) test $(BENCHFLAGS) ./pkg/crypto/... -run='^$$'

bench-all: ## Run all benchmarks across all packages
	$(GO) test $(BENCHFLAGS) $(PKG) -run='^$$'

bench-compare: ## Show instructions for benchmark comparison
	@echo "Run 'go test -bench=. -benchmem ./benchmark/... > new.txt' to create a baseline"
	@echo "Then run 'benchstat old.txt new.txt' to compare"

## Code quality targets

fmt: ## Format code
	$(GO) fmt $(PKG)
	@echo "Code formatted"

vet: ## Run go vet
	$(GO) vet $(PKG)

lint: ## Run golangci-lint (falls back to go vet if not installed)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run $(PKG); \
	else \
		echo "golangci-lint not installed, running go vet instead"; \
		echo "Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		$(GO) vet $(PKG); \
	fi

coverage: test ## Generate HTML coverage report
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## Fuzz testing targets

fuzz: ## Run all fuzz tests (use FUZZTIME=60s to override duration)
	@echo "Running all fuzz tests for $(FUZZTIME)..."
	$(GO) test -fuzz=FuzzDecrypt -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzDecryptRoundTrip -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzNewCipher -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzAddressBookJSON -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzMessageIterator -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzHandshakeMessageParsing -fuzztime=$(FUZZTIME) ./fuzz/

fuzz-crypto: ## Run crypto-related fuzz tests
	@echo "Running crypto fuzz tests for $(FUZZTIME)..."
	$(GO) test -fuzz=FuzzDecrypt -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzDecryptRoundTrip -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzEd25519PublicToX25519 -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzEd25519PrivateToX25519 -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzNewCipher -fuzztime=$(FUZZTIME) ./fuzz/

fuzz-addressbook: ## Run address book fuzz tests
	@echo "Running address book fuzz tests for $(FUZZTIME)..."
	$(GO) test -fuzz=FuzzAddressBookJSON -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzPeerEntryJSON -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzMultiaddrParsing -fuzztime=$(FUZZTIME) ./fuzz/

fuzz-cramberry: ## Run Cramberry message fuzz tests
	@echo "Running Cramberry message fuzz tests for $(FUZZTIME)..."
	$(GO) test -fuzz=FuzzMessageIterator -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzStreamReaderVarint -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzStreamReaderString -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzStreamReaderBytes -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzStreamWriterReader -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzDelimitedMessages -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzMarshalUnmarshal -fuzztime=$(FUZZTIME) ./fuzz/

fuzz-handshake: ## Run handshake protocol fuzz tests
	@echo "Running handshake protocol fuzz tests for $(FUZZTIME)..."
	$(GO) test -fuzz=FuzzHandshakeMessageParsing -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzHandshakeDelimited -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzHandshakeMessageRoundTrip -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzCryptoMaterial -fuzztime=$(FUZZTIME) ./fuzz/
	$(GO) test -fuzz=FuzzMultipleHandshakeMessages -fuzztime=$(FUZZTIME) ./fuzz/

fuzz-list: ## List all available fuzz targets
	@echo "Available fuzz targets:"
	@$(GO) test -list='Fuzz.*' ./fuzz/ 2>/dev/null | grep -E '^Fuzz'

## Utility targets

clean: ## Clean build artifacts and caches
	$(GO) clean $(PKG)
	$(GO) clean -cache -testcache
	rm -f coverage.out coverage.html

tidy: ## Tidy go.mod
	$(GO) mod tidy

deps: ## Download dependencies
	$(GO) mod download

verify: ## Verify dependencies
	$(GO) mod verify

## Example targets

examples: ## Run all example applications
	@echo "\n=== Simple Chat Example ==="
	@echo "Note: This example requires two terminals to run interactively."
	@echo "Run: go run ./examples/simple-chat/"

example-simple-chat: ## Run simple-chat example
	@$(GO) run ./examples/simple-chat/

## Development helpers

check: fmt vet lint test ## Run all checks (format, vet, lint, test)

ci: ## Run CI pipeline locally
	@echo "Running CI pipeline..."
	$(MAKE) deps
	$(MAKE) fmt
	$(MAKE) vet
	$(MAKE) lint
	$(MAKE) test
	$(MAKE) build
	@echo "CI pipeline complete"

## Help

help: ## Show this help
	@echo "Glueberry Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
