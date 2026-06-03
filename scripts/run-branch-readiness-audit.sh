#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT_DIR/docker-compose.prod.yml}"
SQL_FILE="${SQL_FILE:-$ROOT_DIR/scripts/branch_readiness_audit.sql}"
OUT_DIR="${AUDIT_OUT_DIR:-$ROOT_DIR/reports}"
STAMP="$(date +%Y%m%d_%H%M%S)"
OUT_FILE="${AUDIT_OUT_FILE:-$OUT_DIR/branch_readiness_audit_$STAMP.txt}"

mkdir -p "$OUT_DIR"

echo "[AUDIT] branch readiness SQL: $SQL_FILE"
echo "[AUDIT] output: $OUT_FILE"

if [ -n "${DATABASE_URL:-}" ] && command -v psql >/dev/null 2>&1; then
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -At -F '|' < "$SQL_FILE" | tee "$OUT_FILE"
else
  if ! command -v docker >/dev/null 2>&1; then
    echo "[AUDIT][FAIL] docker is required when DATABASE_URL+psql are not available" >&2
    exit 1
  fi
  pg_user="$(docker compose -f "$COMPOSE_FILE" exec -T postgres printenv POSTGRES_USER 2>/dev/null || true)"
  pg_db="$(docker compose -f "$COMPOSE_FILE" exec -T postgres printenv POSTGRES_DB 2>/dev/null || true)"
  pg_user="${pg_user:-turcompany}"
  pg_db="${pg_db:-turcompany}"
  docker compose -f "$COMPOSE_FILE" exec -T postgres \
    psql -v ON_ERROR_STOP=1 -U "$pg_user" -d "$pg_db" -At -F '|' \
    < "$SQL_FILE" | tee "$OUT_FILE"
fi

critical_count="$(awk -F '|' '$2 == "CRITICAL" && ($3 + 0) > 0 { total += ($3 + 0) } END { print total + 0 }' "$OUT_FILE")"
warn_count="$(awk -F '|' '$2 == "WARN" && ($3 + 0) > 0 { total += ($3 + 0) } END { print total + 0 }' "$OUT_FILE")"

echo "[AUDIT] critical unresolved rows: $critical_count"
echo "[AUDIT] warning rows: $warn_count"

if [ "$critical_count" -gt 0 ]; then
  echo "[AUDIT][FAIL] Fix CRITICAL branch readiness rows before production deploy." >&2
  exit 2
fi

echo "[AUDIT][OK] No critical branch readiness gaps found."
