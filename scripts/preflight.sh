#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env.prod}"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT_DIR/docker-compose.prod.yml}"
DRY_RUN="${DRY_RUN:-0}"
SMOKE_ONLY="${SMOKE_ONLY:-0}"
SERVER_DOMAIN="${SERVER_DOMAIN:-kubcrm.kz}"
API_BASE_URL="${API_BASE_URL:-http://localhost:${API_PORT:-4000}}"
SKIP_EMAIL_SMOKE="${SKIP_EMAIL_SMOKE:-0}"

ok_count=0
warn_count=0
fail_count=0
fixes=()

ok(){ echo "[OK] $*"; ok_count=$((ok_count+1)); }
warn(){ echo "[WARN] $*"; warn_count=$((warn_count+1)); fixes+=("$*"); }
fail(){ echo "[FAIL] $*"; fail_count=$((fail_count+1)); fixes+=("$*"); }

is_true(){ case "${1,,}" in 1|true|yes|on) return 0;; *) return 1;; esac; }
require_var(){ local n="$1"; if [[ -z "${!n:-}" ]]; then fail "Заполните $n в .env.prod"; else ok "$n задан"; fi; }

parse_json(){ python3 -c 'import json,sys; d=json.loads(sys.stdin.read());
for k in sys.argv[1].split("."):
 d=d[k]
print(d if d is not None else "")' "$1" 2>/dev/null || true; }

check_tools(){
  command -v docker >/dev/null 2>&1 && ok "docker найден" || fail "Установите Docker"
  if docker compose version >/dev/null 2>&1; then ok "docker compose найден"; else fail "Установите Docker Compose v2"; fi
  command -v curl >/dev/null 2>&1 && ok "curl найден" || fail "Установите curl"
  command -v python3 >/dev/null 2>&1 && ok "python3 найден" || warn "python3 не найден: разбор JSON ограничен"
}

load_env(){
  if [[ ! -f "$ENV_FILE" ]]; then fail "Создайте $ENV_FILE"; return; fi
  set -a; source "$ENV_FILE"; set +a
  ok "Загружен $ENV_FILE"
}

check_env(){
  require_var JWT_SECRET
  if [[ -n "${DATABASE_URL:-}" ]]; then ok "DATABASE_URL задан"; else
    require_var POSTGRES_USER; require_var POSTGRES_PASSWORD; require_var POSTGRES_DB; require_var POSTGRES_HOST
  fi
  require_var TELEGRAM_ENABLE
  if is_true "${TELEGRAM_ENABLE:-false}"; then
    if [[ -z "${TELEGRAM_BOT_TOKEN:-${TELEGRAM_APITOKEN:-}}" ]]; then fail "Заполните TELEGRAM_BOT_TOKEN или TELEGRAM_APITOKEN"; else ok "Telegram token задан"; fi
    if is_true "${TELEGRAM_WEBHOOK_ENABLED:-true}"; then require_var TELEGRAM_WEBHOOK_URL; fi
  fi
  require_var SMTP_HOST; require_var SMTP_PORT; require_var SMTP_USER; require_var SMTP_PASSWORD
  if [[ -z "${EMAIL_FROM:-${SMTP_FROM:-}}" ]]; then fail "Заполните EMAIL_FROM (или SMTP_FROM)"; else ok "EMAIL_FROM/SMTP_FROM задан"; fi
  require_var SIGN_EMAIL_TOKEN_PEPPER
  require_var SIGN_EMAIL_VERIFY_BASE_URL
  require_var SIGN_BASE_URL
}

check_compose_api_env(){
  local api_block
  api_block=$(awk '/^  api:/{flag=1} /^  [a-zA-Z0-9_-]+:/{if(flag&&$1!="api:")flag=0} flag{print}' "$COMPOSE_FILE")
  echo "$api_block" | grep -q 'DATABASE_URL' && ok "api получает DATABASE_URL" || fail "Добавьте DATABASE_URL в environment api"
  echo "$api_block" | grep -q 'CONFIG_PATH' && ok "api получает CONFIG_PATH" || fail "Добавьте CONFIG_PATH в environment api"
}

start_stack(){
  if [[ "$DRY_RUN" == "1" || "$SMOKE_ONLY" == "1" ]]; then warn "DRY_RUN/SMOKE_ONLY: пропуск поднятия стека"; return; fi
  (cd "$ROOT_DIR" && docker compose -f "$COMPOSE_FILE" up -d --build)
  ok "Стек поднят"
  for _ in {1..40}; do
    status=$(cd "$ROOT_DIR" && docker compose -f "$COMPOSE_FILE" ps --format json api 2>/dev/null | python3 -c 'import json,sys; t=sys.stdin.read().strip();
print((json.loads(t)[0].get("Health") if t.startswith("[") else json.loads(t).get("Health","")) if t else "")' 2>/dev/null || true)
    [[ "$status" == "healthy" ]] && ok "api healthcheck=healthy" && return
    sleep 3
  done
  fail "api не стал healthy"
}

run_migrations_and_db_checks(){
  if [[ "$DRY_RUN" == "1" ]]; then warn "DRY_RUN: пропуск миграций и БД-проверок"; return; fi
  if (cd "$ROOT_DIR" && docker compose -f "$COMPOSE_FILE" up -d postgres >/dev/null && docker compose -f "$COMPOSE_FILE" run --rm migrate >/tmp/preflight_migrate.log 2>&1); then
    ok "Миграции выполнены"
  else
    fail "Проверьте миграции: docker compose -f docker-compose.prod.yml run --rm migrate"
    cat /tmp/preflight_migrate.log || true
  fi

  local dsn="${DATABASE_URL:-postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST:-postgres}:${POSTGRES_PORT:-5432}/${POSTGRES_DB}?sslmode=disable}"
  local sql="select table_name from information_schema.tables where table_schema='public' and table_name in ('telegram_links','signature_confirmations','sign_sessions') order by table_name;"
  out=$(cd "$ROOT_DIR" && docker compose -f "$COMPOSE_FILE" exec -T postgres psql "$dsn" -At -c "$sql" 2>/dev/null || true)
  [[ "$out" == *"telegram_links"* ]] && ok "Таблица telegram_links есть" || fail "Отсутствует таблица telegram_links"
  [[ "$out" == *"signature_confirmations"* ]] && ok "Таблица signature_confirmations есть" || fail "Отсутствует таблица signature_confirmations"
  [[ "$out" == *"sign_sessions"* ]] && ok "Таблица sign_sessions есть" || warn "sign_sessions не найдена (проверьте схему)"
}

http_status(){ curl -sS -o /tmp/preflight_body.json -w "%{http_code}" "$@" || echo 000; }

run_smoke(){
  local code
  code=$(http_status "$API_BASE_URL/healthz")
  [[ "$code" == "200" ]] && ok "GET /healthz = 200" || fail "GET /healthz вернул $code"

  local jwt=""
  if [[ -n "${SMOKE_LOGIN_EMAIL:-}" && -n "${SMOKE_LOGIN_PASSWORD:-}" ]]; then
    code=$(curl -sS -o /tmp/preflight_login.json -w "%{http_code}" -H 'Content-Type: application/json' -d "{\"email\":\"$SMOKE_LOGIN_EMAIL\",\"password\":\"$SMOKE_LOGIN_PASSWORD\"}" "$API_BASE_URL/auth/login" || echo 000)
    if [[ "$code" == "200" ]]; then
      jwt=$(cat /tmp/preflight_login.json | parse_json 'tokens.access_token')
      [[ -n "$jwt" ]] && ok "POST /auth/login = 200" || warn "Login успешен, но access_token не извлечен"
    else
      fail "POST /auth/login вернул $code"
    fi
  else
    warn "SMOKE_LOGIN_EMAIL/SMOKE_LOGIN_PASSWORD не заданы: login smoke пропущен"
  fi

  if is_true "${TELEGRAM_ENABLE:-false}"; then
    if [[ -z "$jwt" ]]; then warn "Telegram smoke пропущен: нет JWT"; else
      code=$(curl -sS -o /tmp/preflight_tg_req.json -w "%{http_code}" -H "Authorization: Bearer $jwt" -X POST "$API_BASE_URL/integrations/telegram/request-link" || echo 000)
      if [[ "$code" == "200" ]]; then
        tg_code=$(cat /tmp/preflight_tg_req.json | parse_json 'code')
        if [[ "$tg_code" =~ ^[0-9A-F]{32}$ ]]; then ok "Telegram code валиден"; else fail "Telegram code имеет неверный формат"; fi
        curl -sS -o /tmp/preflight_tg_webhook.json -w "%{http_code}" -H 'Content-Type: application/json' -d "{\"update_id\":1,\"message\":{\"message_id\":1,\"chat\":{\"id\":12345,\"type\":\"private\"},\"text\":\"/start $tg_code\"}}" "$API_BASE_URL/integrations/telegram/webhook" >/tmp/preflight_tg_webhook.status || true
        code=$(curl -sS -o /tmp/preflight_tg_link.json -w "%{http_code}" -H "Authorization: Bearer $jwt" "$API_BASE_URL/integrations/telegram/link?code=$tg_code" || echo 000)
        [[ "$code" == "200" ]] && ok "Telegram link flow завершен" || fail "GET /integrations/telegram/link вернул $code"
      else
        fail "POST /integrations/telegram/request-link вернул $code"
      fi
    fi
  fi

  if [[ "$SKIP_EMAIL_SMOKE" == "1" ]]; then
    warn "SKIP_EMAIL_SMOKE=1: email smoke пропущен"
  elif [[ -n "${SMOKE_SIGN_DOC_ID:-}" && -n "$jwt" && -n "${SMOKE_EMAIL_TOKEN:-}" && -n "${SMOKE_EMAIL_CODE:-}" ]]; then
    code=$(curl -sS -o /tmp/preflight_email_start.json -w "%{http_code}" -H "Authorization: Bearer $jwt" -H 'Content-Type: application/json' -X POST "$API_BASE_URL/documents/${SMOKE_SIGN_DOC_ID}/sign/start" -d '{}' || echo 000)
    [[ "$code" == "200" ]] && ok "POST /documents/{id}/sign/start = 200" || fail "sign/start вернул $code"
    code=$(curl -sS -o /tmp/preflight_email_verify.json -w "%{http_code}" "$API_BASE_URL/sign/email/verify?token=${SMOKE_EMAIL_TOKEN}" || echo 000)
    [[ "$code" == "200" ]] && ok "GET /sign/email/verify = 200" || fail "email/verify вернул $code"
    code=$(curl -sS -o /tmp/preflight_email_confirm.json -w "%{http_code}" -H "Authorization: Bearer $jwt" -H 'Content-Type: application/json' -X POST "$API_BASE_URL/documents/${SMOKE_SIGN_DOC_ID}/sign/confirm/email" -d "{\"token\":\"${SMOKE_EMAIL_TOKEN}\",\"code\":\"${SMOKE_EMAIL_CODE}\"}" || echo 000)
    [[ "$code" == "200" ]] && ok "POST /documents/{id}/sign/confirm/email = 200" || fail "sign/confirm/email вернул $code"
  else
    warn "Email smoke пропущен: задайте SMOKE_SIGN_DOC_ID, SMOKE_EMAIL_TOKEN, SMOKE_EMAIL_CODE или SKIP_EMAIL_SMOKE=1"
  fi
}

check_cors_and_webhook(){
  local cfg_path="${CONFIG_PATH:-$ROOT_DIR/config/config.yaml}"
  if [[ -f "$cfg_path" ]]; then
    if rg -n "allow_origins" "$cfg_path" >/dev/null 2>&1 && rg -n "$SERVER_DOMAIN" "$cfg_path" >/dev/null 2>&1; then
      ok "CORS allow_origins содержит $SERVER_DOMAIN"
    else
      warn "Проверьте cors.allow_origins в $cfg_path и добавьте домен $SERVER_DOMAIN"
    fi
  else
    warn "Файл конфига $cfg_path не найден для проверки CORS"
  fi

  if is_true "${TELEGRAM_ENABLE:-false}"; then
    if [[ "${TELEGRAM_WEBHOOK_URL:-}" == *"ngrok"* && "$SERVER_DOMAIN" != *"localhost"* ]]; then
      warn "TELEGRAM_WEBHOOK_URL указывает на ngrok, для прод используйте домен сервера"
    elif [[ "${TELEGRAM_WEBHOOK_URL:-}" == https://* ]]; then
      ok "TELEGRAM_WEBHOOK_URL выглядит корректно"
    else
      fail "Проверьте TELEGRAM_WEBHOOK_URL: ожидается HTTPS URL"
    fi
  fi
}

print_summary(){
  echo "\n===== PREFLIGHT REPORT ====="
  echo "OK: $ok_count"
  echo "WARN: $warn_count"
  echo "FAIL: $fail_count"
  if (( ${#fixes[@]} > 0 )); then
    echo "\nРекомендации по исправлению:"
    for f in "${fixes[@]}"; do echo " - $f"; done
  fi
  if (( fail_count > 0 )); then exit 1; fi
}

check_tools
load_env
check_env
check_compose_api_env
check_cors_and_webhook
start_stack
run_migrations_and_db_checks
if [[ "$DRY_RUN" == "1" ]]; then
  warn "DRY_RUN: smoke тесты пропущены"
else
  run_smoke
fi
print_summary
