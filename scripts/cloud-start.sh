#!/usr/bin/env bash
# Per-boot runtime reconciliation for Cursor Cloud Agents.
# Starts local infrastructure, ensures the app database exists, and applies migrations.
# Must be safe to run repeatedly.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Start local infrastructure (sysvinit scripts work without systemd).
sudo service postgresql start
sudo service redis-server start

# Wait for PostgreSQL to accept connections.
for _ in $(seq 1 30); do
  if sudo -u postgres pg_isready >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

# Ensure the application role and database exist (matches docker-compose defaults).
if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='tokenhub'" | grep -q 1; then
  sudo -u postgres psql -c "CREATE ROLE tokenhub LOGIN PASSWORD 'tokenhub';"
fi
if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='tokenhub'" | grep -q 1; then
  sudo -u postgres createdb -O tokenhub tokenhub
fi

# Apply database migrations (idempotent).
set -a
# shellcheck disable=SC1091
source .env
set +a
( cd backend && go run ./cmd/migrate )

echo "cloud-start: done"
