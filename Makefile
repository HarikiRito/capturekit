.PHONY: fmt lint test build check

fmt:
	gofmt -w .
	@golangci-lint fmt ./... 2>/dev/null || true

lint:
	golangci-lint run ./...

test:
	go test -race ./...

build:
	go build -o bin/capture ./cmd/capture

check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt needed"; exit 1; }
	$(MAKE) lint
	$(MAKE) test
