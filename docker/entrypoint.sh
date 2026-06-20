#!/bin/sh
set -eu

: "${FYOM_DB_PATH:=/app/data/fyom.db}"

mkdir -p /app/data

# Best-effort ownership fix for named volumes and bind mounts.
# This may fail on read-only or restricted filesystems, so keep the error visible but non-fatal.
if [ "$(id -u)" = "0" ]; then
  chown -R 10001:10001 /app/data 2>/dev/null || {
    echo "warning: unable to chown /app/data; continuing" >&2
  }

  exec su-exec fyom:fyom "$@"
fi

exec "$@"
