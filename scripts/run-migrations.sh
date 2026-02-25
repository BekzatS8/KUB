#!/bin/sh
set -eu

: "${POSTGRES_USER:?POSTGRES_USER is required}"
: "${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}"
: "${POSTGRES_DB:?POSTGRES_DB is required}"

set -- /migrations/*_*.up.sql
if [ ! -e "$1" ]; then
  echo "No up migration files found in /migrations. Nothing to apply."
  exit 0
fi

for file in $(ls /migrations/*_*.up.sql | sort); do
  echo "Applying migration: $file"
  PGPASSWORD="$POSTGRES_PASSWORD" psql \
    -v ON_ERROR_STOP=1 \
    -h postgres \
    -U "$POSTGRES_USER" \
    -d "$POSTGRES_DB" \
    -f "$file"
done

echo "Migrations completed successfully."
