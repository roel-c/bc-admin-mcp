.PHONY: build run run-http smoke smoke-msf test vet lint lint-install clean

BINARY := bc-mcp-server
CMD    := ./cmd/server

# Pinned so local runs match CI (.github/workflows/ci.yml). Bump both together.
GOLANGCI_LINT_VERSION := v1.64.8

build:
	go build -buildvcs=false -o $(BINARY) $(CMD)

# Full pre-session live smoke: one R0 read per domain. Requires .env with BC_* credentials.
smoke:
	./scripts/smoke_all_domains.sh

# MSF-only live checks (channels, assignments, trees, listings). Requires .env with BC_* credentials.
smoke-msf:
	./scripts/smoke_msf_slice.sh

run: build
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi && ./$(BINARY)

run-http: build
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi && MCP_TRANSPORT=streamable-http ./$(BINARY)

test:
	go test ./... -v -count=1

vet:
	go vet ./...

# Install the pinned golangci-lint version.
lint-install:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not installed. Run: make lint-install"; \
		exit 1; \
	}
	@golangci-lint version 2>/dev/null | grep -q "$(GOLANGCI_LINT_VERSION:v%=%)" || \
		echo "warning: installed golangci-lint differs from pinned $(GOLANGCI_LINT_VERSION) — results may vary (run: make lint-install)"
	golangci-lint run ./...

clean:
	rm -f $(BINARY)
