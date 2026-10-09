.PHONY: build up migrate bootstrap test down
build:
	docker compose build
up:
	docker compose up -d --build
migrate:
	docker compose run --rm migrate
bootstrap:
	docker compose --profile setup run --rm bootstrap
test:
	cd backend && go test -race ./... && go vet ./...
	cd frontend && npm ci && npm run build
down:
	docker compose down
