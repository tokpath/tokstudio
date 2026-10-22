#!/bin/sh
set -eu

output=${1:?Pass the release binary output directory}
mkdir -p "$output"

# Bifrost's datasheet dependency includes CGO SQLite error types. Link that
# dependency statically so the existing distroless runtime needs no shared libs.
# TokenHub continues to use PostgreSQL, with Go's DNS and user lookup helpers.
for command in api worker migrate seed-demo; do
  CGO_ENABLED=1 GOOS=linux go build -tags netgo,osusergo,sqlite_omit_load_extension \
    -ldflags '-linkmode external -extldflags "-static"' \
    -o "$output/$command" "./cmd/$command"
done

# The backup image uses Alpine and has no Bifrost dependency.
CGO_ENABLED=0 GOOS=linux go build -o "$output/backup" ./cmd/backup
