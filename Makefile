.PHONY: all web build run dev test clean

all: build

web:
	cd web && npm ci && npm run build

build: web
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/lumen ./cmd/lumen

# Backend only (uses the last built web client).
server:
	CGO_ENABLED=0 go build -o bin/lumen ./cmd/lumen

run: server
	./bin/lumen

# Frontend dev server with hot reload (proxies /api to a backend on :8097).
dev:
	cd web && LUMEN_API=http://localhost:8097 npm run dev

test:
	go vet ./...
	go test ./...
	cd web && npx tsc --noEmit -p .

clean:
	rm -rf bin web/dist/assets
