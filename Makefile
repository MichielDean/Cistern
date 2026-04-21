.PHONY: web-build web-dev build test lint

web-build:
	cd web && npm install && npm run build

web-dev:
	cd web && npm run dev

build:
	go build ./...

test:
	go test ./...

lint:
	go vet ./...