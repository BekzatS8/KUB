# Wazzup / WhatsApp integration

## Security first
- **Do not commit real `WAZZUP_API_TOKEN` to git.**
- Keep real secrets only in `.env.local` (untracked), server secret storage, or CI secrets.
- API token is read from config/env at runtime and is never returned via API.

## Required config when enabled
If `wazzup.enable=true`, these fields are required:
- `wazzup.api_base_url`
- `wazzup.api_token`

## White Label: аккаунт добавления каналов ДОЛЖЕН совпадать с рабочим

Это самая частая ловушка в этой интеграции.

Вся работа CRM идёт под `WAZZUP_API_TOKEN`: список каналов (`GET /v3/channels`),
отправка сообщений, вебхуки входящих. А кнопка «Добавить канал» открывает iframe
партнёрского (tech-partner) аккаунта и подключает канал в тот аккаунт Wazzup,
id которого указан в `WAZZUP_WL_ACCOUNT_ID`.

Если это РАЗНЫЕ аккаунты (например сервис работает на основном аккаунте, а в
White Label указана его «дочка»), то канал, подключённый через кнопку:
- не появится в «Каналах мессенджера» даже после «Обновить» — синхронизация
  спрашивает каналы у основного аккаунта;
- не будет присылать входящие — вебхук настроен на основном аккаунте;
- не сможет использоваться для отправки.

Правильно — один из двух вариантов:
1. `WAZZUP_WL_ACCOUNT_ID` = id того же аккаунта, чей токен лежит в
   `WAZZUP_API_TOKEN`. Тогда кнопка «Добавить канал» работает как задумано.
2. Не использовать кнопку: подключать каналы в кабинете Wazzup рабочего
   аккаунта и нажимать «Обновить» в CRM.

Перевести CRM на дочерний аккаунт тоже можно, но это смена `WAZZUP_API_TOKEN`
и повторный `POST /integrations/wazzup/setup` (перенастройка вебхуков). Каналы,
чаты и лиды, заведённые на старом аккаунте, при этом не переезжают.

## Config fields
- `wazzup.enable` — enable/disable integration wiring.
- `wazzup.api_base_url` — provider API base URL (default: `https://api.wazzup24.com`).
- `wazzup.api_token` — provider API token (secret).
- `wazzup.channel_id` — optional channel/source identifier for outbound sends.
- `wazzup.webhook_verify_token` — optional extra verification token for webhook Authorization.
- `wazzup.webhook_base_url` — base URL used by setup endpoint to build callback URL.
- `wazzup.request_timeout_sec` — outbound request timeout.
- `wazzup.retry_count` — retry count for 5xx/429/network errors.
- `wazzup.retry_delay_ms` — delay between retries.

All fields can be set via env (`WAZZUP_*`) or yaml config.

## Local enable flow (safe)
1. Copy local env template:
   - `cp .env.local.example .env.local`
2. Set local env values (do not commit `.env.local`):
   - `WAZZUP_ENABLE=true`
   - `WAZZUP_API_TOKEN=<your_real_token>`
   - `WAZZUP_WEBHOOK_BASE_URL=http://localhost:4000`
3. Start app with local config (`CONFIG_PATH=config/config.local.example.yaml` or local config file).
4. Run setup endpoint once (JWT required, any authenticated known role):
   - `POST /integrations/wazzup/setup` with `{"webhooks_base_url":"http://localhost:4000","enabled":true}`

## Implemented endpoints
Public:
- `POST /integrations/wazzup/webhook/:token`
- `GET /integrations/wazzup/crm/:token/users`
- `GET /integrations/wazzup/crm/:token/users/:id`

JWT protected:
- `POST /integrations/wazzup/setup` (any authenticated known role)
- `POST /integrations/wazzup/iframe` (global iframe; payload `{}` — `lead_id/client_id` не используются)
- `POST /integrations/wazzup/send`

## Quick checks
### Send path
```bash
curl -X POST http://localhost:4000/integrations/wazzup/send \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{"chat_id":"77001112233","text":"test from KUB"}'
```

### Webhook path
```bash
curl -X POST http://localhost:4000/integrations/wazzup/webhook/<webhook_token> \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <WAZZUP_WEBHOOK_VERIFY_TOKEN>" \
  -d '{"messages":[{"messageId":"m-1","chatType":"whatsapp","chatId":"77001112233","text":"hello"}]}'
```

## Current scope and TODO
Implemented in this step:
- secure runtime config,
- typed API client with timeout/retry/auth header,
- setup/iframe/send/webhook flows with safer logging.

Not implemented intentionally:
- full omnichannel router,
- template catalog sync,
- advanced webhook signature schemes beyond verify token + integration token.
