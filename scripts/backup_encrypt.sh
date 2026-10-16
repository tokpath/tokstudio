#!/usr/bin/env bash
# Environment files are parsed by Go; never executed as shell code.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKUP_DIR="${TOKENHUB_BACKUP_DIR:?TOKENHUB_BACKUP_DIR required}"
umask 077
mkdir -p "$BACKUP_DIR"
OUT="$BACKUP_DIR/tokenhub-$(date -u +%Y%m%dT%H%M%SZ)_$$.thbk"
cd "$ROOT/backend"
exec go run ./cmd/backup --mode=create --output="$OUT"
