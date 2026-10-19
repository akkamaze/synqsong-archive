GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

.PHONY: check test lint

check: lint test

test:
	go test -race ./...

lint:
	$(GOLANGCI_LINT) run ./...
