.PHONY: all build test test-race lint clean benchmark bench

all: build test lint

build:
	go build ./...

test:
	go test -race -v ./...

test-race:
	go test -race -v ./...

test-short:
	go test -short -v ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, running go vet instead"; \
		go vet ./...; \
	fi

clean:
	go clean ./...
	rm -f coverage.out

coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

fmt:
	go fmt ./...

tidy:
	go mod tidy

# Benchmark targets
benchmark: bench

bench:
	go test -bench=. -benchmem ./benchmark/...
	go test -bench=. -benchmem ./pkg/crypto/...

bench-crypto:
	go test -bench=. -benchmem ./benchmark/... -run='^$$'
	go test -bench=. -benchmem ./pkg/crypto/... -run='^$$'

bench-all:
	go test -bench=. -benchmem ./... -run='^$$'

bench-compare:
	@echo "Run 'go test -bench=. -benchmem ./benchmark/... > new.txt' to create a baseline"
	@echo "Then run 'benchstat old.txt new.txt' to compare"
