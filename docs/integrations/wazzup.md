# Wazzup / WhatsApp integration

## Security first
- **Do not commit real `WAZZUP_API_TOKEN` to git.**
- Keep real secrets only in `.env.local` (untracked), server secret storage, or CI secrets.
- API token is read from config/env at runtime and is never returned via API.

## Required config when enabled
If `wazzup.enable=true`, these fields are required:
- `wazzup.api_base_url`
- `wazzup.api_token`

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
4. Run setup endpoint once (JWT `system_admin` required):
   - `POST /integrations/wazzup/setup` with `{"webhooks_base_url":"http://localhost:4000","enabled":true}`

## Implemented endpoints
Public:
- `POST /integrations/wazzup/webhook/:token`
- `GET /integrations/wazzup/crm/:token/users`
- `GET /integrations/wazzup/crm/:token/users/:id`

JWT protected:
- `POST /integrations/wazzup/setup` (`system_admin` only)
- `POST /integrations/wazzup/iframe`
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
