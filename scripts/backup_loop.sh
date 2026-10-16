#!/bin/sh
# Authenticated database + durable private-object backups; no model content.
set -eu
: "${TOKENHUB_BACKUP_KEY:?configure a separate random backup key}"
[ "${#TOKENHUB_BACKUP_KEY}" -ge 32 ] || { echo 'backup key must be at least 32 characters' >&2; exit 1; }
BACKUP_DIR="${TOKENHUB_BACKUP_DIR:?configure backup destination}"
INTERVAL="${TOKENHUB_BACKUP_INTERVAL_SECONDS:-86400}"
case "$INTERVAL" in ''|*[!0-9]*) echo 'invalid backup interval' >&2; exit 1;; esac
[ "$INTERVAL" -ge 60 ] || exit 1
umask 077
mkdir -p "$BACKUP_DIR"
while true; do
  STAMP="$(date -u +%Y%m%dT%H%M%SZ)_$$"
  if tokenhub-backup --mode=create --output="$BACKUP_DIR/tokenhub-$STAMP.thbk"; then
    find "$BACKUP_DIR" -type f -name 'tokenhub-*.thbk' -mtime +30 -delete
  else
    echo 'backup failed; prior backups retained; operator follow-up required' >&2
  fi
  sleep "$INTERVAL"
done
