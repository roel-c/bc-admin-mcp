.PHONY: build run run-http smoke smoke-msf test vet lint clean

BINARY := bc-mcp-server
CMD    := ./cmd/server

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

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY)
