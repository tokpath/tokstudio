#!/usr/bin/env bash
# A drill succeeds only when the archive, database, objects and ledger verify.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec bash "$ROOT/scripts/restore_drill.sh" "$@"
