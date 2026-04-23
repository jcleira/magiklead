.PHONY: dev dev-infra api worker fe build migrate lint stop

# Start everything (backend + frontend + infra)
dev: dev-infra
	@echo "Starting backend on :8040 and frontend on :3040..."
	@make api &
	@make fe &
	@wait

dev-infra:
	docker compose up -d

api:
	cd backend && go run ./cmd/api/...

worker:
	cd backend && go run ./cmd/worker/...

fe:
	cd frontend && pnpm dev

stop:
	@lsof -i :8040 -t 2>/dev/null | xargs kill 2>/dev/null || true
	@lsof -i :3040 -t 2>/dev/null | xargs kill 2>/dev/null || true
	@echo "Stopped."

migrate:
	cd backend && go run ./cmd/migrate/...

build:
	cd backend && go build -o bin/api ./cmd/api/...
	cd backend && go build -o bin/worker ./cmd/worker/...
	cd frontend && pnpm build

lint:
	cd backend && go vet ./...
	cd frontend && pnpm lint
