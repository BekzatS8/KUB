#!/bin/sh
set -e

if [ -n "${DATABASE_URL:-}" ]; then
  retries="${DB_WAIT_RETRIES:-30}"
  interval="${DB_WAIT_INTERVAL:-2}"
  echo "[BOOT] waiting for postgres (${retries} retries, ${interval}s interval)"
  i=1
  while [ "$i" -le "$retries" ]; do
    if pg_isready -d "$DATABASE_URL" >/dev/null 2>&1; then
      echo "[BOOT] postgres is ready"
      break
    fi
    if [ "$i" -eq "$retries" ]; then
      echo "[BOOT] postgres is not ready after ${retries} attempts" >&2
      exit 1
    fi
    i=$((i + 1))
    sleep "$interval"
  done
fi

exec "$@"
