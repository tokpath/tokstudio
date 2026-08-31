#!/usr/bin/env bash
# Idempotent repository bootstrap for Cursor Cloud Agents.
# Runs after checkout (at build time when environment builds are enabled). Must terminate.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Local dev config. .env is gitignored, so create it from the committed template.
if [[ ! -f .env ]]; then
  cp .env.example .env
fi

# Backend Go modules.
( cd backend && go mod download )

# Web dependencies (clean, reproducible install from the lockfile).
( cd web && npm ci )

echo "cloud-install: done"
