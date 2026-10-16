#!/usr/bin/env bash
# Target database must already exist, be empty, and use the drill prefix.
# Configure S3_* for a separate empty private destination bucket.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
: "${TOKENHUB_RESTORE_DATABASE_URL:?explicit isolated target database required}"
backup_input="${1:-${TOKENHUB_RESTORE_BACKUP_FILE:-}}"
: "${backup_input:?explicit backup filename argument or TOKENHUB_RESTORE_BACKUP_FILE required}"
cd "$ROOT/backend"
exec go run ./cmd/backup --mode=restore-drill --input="$backup_input"
