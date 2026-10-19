GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

.PHONY: check test lint db-up db-down

check: lint test

test:
	go test -race ./...

lint:
	$(GOLANGCI_LINT) run ./...

# Needs a filled-in .env (see .env.example).
db-up:
	docker compose up -d --wait

db-down:
	docker compose down
