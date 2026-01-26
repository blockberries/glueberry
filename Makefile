.PHONY: all build test test-race lint clean benchmark bench fuzz fuzz-crypto fuzz-addressbook fuzz-cramberry fuzz-handshake

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

# Fuzz testing targets
# Default fuzz time is 30 seconds; override with FUZZTIME=60s make fuzz
FUZZTIME ?= 30s

fuzz:
	@echo "Running all fuzz tests for $(FUZZTIME)..."
	go test -fuzz=FuzzDecrypt -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzDecryptRoundTrip -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzNewCipher -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzAddressBookJSON -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzMessageIterator -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzHandshakeMessageParsing -fuzztime=$(FUZZTIME) ./fuzz/

fuzz-crypto:
	@echo "Running crypto fuzz tests for $(FUZZTIME)..."
	go test -fuzz=FuzzDecrypt -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzDecryptRoundTrip -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzEd25519PublicToX25519 -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzEd25519PrivateToX25519 -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzNewCipher -fuzztime=$(FUZZTIME) ./fuzz/

fuzz-addressbook:
	@echo "Running address book fuzz tests for $(FUZZTIME)..."
	go test -fuzz=FuzzAddressBookJSON -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzPeerEntryJSON -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzMultiaddrParsing -fuzztime=$(FUZZTIME) ./fuzz/

fuzz-cramberry:
	@echo "Running Cramberry message fuzz tests for $(FUZZTIME)..."
	go test -fuzz=FuzzMessageIterator -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzStreamReaderVarint -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzStreamReaderString -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzStreamReaderBytes -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzStreamWriterReader -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzDelimitedMessages -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzMarshalUnmarshal -fuzztime=$(FUZZTIME) ./fuzz/

fuzz-handshake:
	@echo "Running handshake protocol fuzz tests for $(FUZZTIME)..."
	go test -fuzz=FuzzHandshakeMessageParsing -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzHandshakeDelimited -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzHandshakeMessageRoundTrip -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzCryptoMaterial -fuzztime=$(FUZZTIME) ./fuzz/
	go test -fuzz=FuzzMultipleHandshakeMessages -fuzztime=$(FUZZTIME) ./fuzz/

fuzz-list:
	@echo "Available fuzz targets:"
	@go test -list='Fuzz.*' ./fuzz/ 2>/dev/null | grep -E '^Fuzz'
