.PHONY: build build-frontend build-backend test test-go test-frontend lint dev clean

build: build-frontend build-backend

build-frontend:
	if [ -d web ]; then cd web && bun install && bun run build; fi

build-backend:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o meadowlark ./cmd/meadowlark

test: test-go test-frontend

test-go:
	go test -race ./...

test-frontend:
	if [ -d web ]; then cd web && bun run test; fi

lint:
	go vet ./...
	if [ -d web ]; then cd web && bunx biome check .; fi

dev:
	# Run both frontend dev server and Go binary in parallel
	# (implementation detail: use a process manager or two terminals)

clean:
	rm -f meadowlark
	rm -rf web/dist web/node_modules
