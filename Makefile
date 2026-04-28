.PHONY: build run test vet lint clean

BINARY := bc-mcp-server
CMD    := ./cmd/server

build:
	go build -buildvcs=false -o $(BINARY) $(CMD)

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
