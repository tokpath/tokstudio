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
if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='tokenhub_v2'" | grep -q 1; then
  sudo -u postgres createdb -O tokenhub tokenhub_v2
fi

# Apply database migrations (idempotent).
# The application parses .env literally. Never execute a credential file as shell.
# Configure the fresh database, private S3 bucket and initial identity first.
( cd backend && go run ./cmd/migrate )

echo "cloud-start: done"
