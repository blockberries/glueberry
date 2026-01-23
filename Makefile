.PHONY: all build test test-race lint clean

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
