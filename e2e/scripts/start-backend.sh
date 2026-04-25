#!/usr/bin/env bash
# Hermetic backend launcher used by the Playwright webServer.
# Wipes the runtime SQLite DB before each cold start so tests are deterministic.
set -euo pipefail

: "${E2E_BACKEND_PORT:?E2E_BACKEND_PORT must be set}"
: "${E2E_FRONTEND_PORT:?E2E_FRONTEND_PORT must be set}"
: "${E2E_RUNTIME_DIR:?E2E_RUNTIME_DIR must be set}"
: "${E2E_ADMIN_EMAIL:?E2E_ADMIN_EMAIL must be set}"
: "${E2E_ADMIN_PASSWORD:?E2E_ADMIN_PASSWORD must be set}"

mkdir -p "$E2E_RUNTIME_DIR"
DB_PATH="$E2E_RUNTIME_DIR/e2e.db"
rm -f "$DB_PATH" "$DB_PATH-wal" "$DB_PATH-shm"

# Generate a per-run master key (32 bytes, base64) for at-rest encryption.
PMS_MASTER_KEY="$(openssl rand -base64 32)"

cd backend
exec env \
  PMS_ENV=test \
  HTTP_ADDR=":${E2E_BACKEND_PORT}" \
  DATABASE_PATH="$DB_PATH" \
  DATA_DIR="$E2E_RUNTIME_DIR/data" \
  CORS_ORIGINS="http://127.0.0.1:${E2E_FRONTEND_PORT}" \
  PMS_MASTER_KEY="$PMS_MASTER_KEY" \
  PMS_COOKIE_SECURE=false \
  PMS_COOKIE_SAMESITE=lax \
  FIRST_SUPERADMIN_EMAIL="$E2E_ADMIN_EMAIL" \
  FIRST_SUPERADMIN_PASSWORD="$E2E_ADMIN_PASSWORD" \
  OCCUPANCY_SYNC_INTERVAL_MINUTES=1440 \
  NUKI_CLEANUP_INTERVAL_MINUTES=1440 \
  CLEANING_RECONCILE_INTERVAL_MINUTES=1440 \
  PMS_BACKUP_INTERVAL_MINUTES=1440 \
  go run ./cmd/server
