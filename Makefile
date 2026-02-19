.PHONY: build build-frontend build-backend test test-go test-frontend lint dev clean

build: build-frontend build-backend

build-frontend:
	cd web && bun install && bun run build

build-backend:
	go build -ldflags="-s -w" -o meadowlark ./cmd/meadowlark

test: test-go test-frontend

test-go:
	go test -race ./...

test-frontend:
	cd web && bun run test

lint:
	go vet ./...
	cd web && bunx biome check .

dev:
	# Run both frontend dev server and Go binary in parallel
	# (implementation detail: use a process manager or two terminals)

clean:
	rm -f meadowlark
	rm -rf web/dist web/node_modules
