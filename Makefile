.PHONY: help setup install deps backend-tidy backend-build backend-test backend-run frontend-install frontend-build frontend-dev frontend-test test build fmt clean

BACKEND_DIR := backend
FRONTEND_DIR := frontend

# Override in shell if needed, e.g.:
# FIRST_SUPERADMIN_EMAIL=admin@example.com FIRST_SUPERADMIN_PASSWORD=secret make backend-run
HTTP_ADDR ?= :8080
DATABASE_PATH ?= ./data/pms.db
SESSION_TTL_HOURS ?= 168
OCCUPANCY_SYNC_INTERVAL_MINUTES ?= 60

help:
	@echo "Available targets:"
	@echo "  setup            - Install frontend deps and tidy Go modules"
	@echo "  backend-build    - Build backend binary"
	@echo "  backend-test     - Run backend tests"
	@echo "  backend-run      - Run backend server locally"
	@echo "  frontend-install - Install frontend npm dependencies"
	@echo "  frontend-build   - Build frontend production bundle"
	@echo "  frontend-dev     - Run Vite dev server"
	@echo "  frontend-test    - Run frontend Vitest suite"
	@echo "  test             - Run backend + frontend tests"
	@echo "  build            - Build backend and frontend"
	@echo "  clean            - Remove build artifacts"

setup: frontend-install backend-tidy

install: setup
deps: setup

backend-tidy:
	cd $(BACKEND_DIR) && go mod tidy

backend-build:
	mkdir -p bin
	cd $(BACKEND_DIR) && go build -o ../bin/pms-server ./cmd/server/

backend-test:
	cd $(BACKEND_DIR) && go test ./...

backend-run:
	mkdir -p data
	cd $(BACKEND_DIR) && \
		HTTP_ADDR="$(HTTP_ADDR)" \
		DATABASE_PATH="$(DATABASE_PATH)" \
		SESSION_TTL_HOURS="$(SESSION_TTL_HOURS)" \
		OCCUPANCY_SYNC_INTERVAL_MINUTES="$(OCCUPANCY_SYNC_INTERVAL_MINUTES)" \
		FIRST_SUPERADMIN_EMAIL="$(FIRST_SUPERADMIN_EMAIL)" \
		FIRST_SUPERADMIN_PASSWORD="$(FIRST_SUPERADMIN_PASSWORD)" \
		go run ./cmd/server/

frontend-install:
	cd $(FRONTEND_DIR) && npm install

frontend-build:
	cd $(FRONTEND_DIR) && npm run build

frontend-dev:
	cd $(FRONTEND_DIR) && npm run dev

frontend-test:
	cd $(FRONTEND_DIR) && npm test

test: backend-test frontend-test

build: backend-build frontend-build

fmt:
	cd $(BACKEND_DIR) && go fmt ./...

clean:
	rm -rf bin
	rm -rf $(FRONTEND_DIR)/dist
