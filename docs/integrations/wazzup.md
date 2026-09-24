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

### WL-токен НЕ является API-ключом (проверено 24.09.2026)

Частое заблуждение при настройке дочернего аккаунта: кажется, что раз
`WhiteLabelClient` уже получает `client_access_token` дочки, то его можно
подставить в `WAZZUP_API_TOKEN` и всё заработает. **Нельзя.** Это два разных
контура авторизации:

| Контур | База | Чем авторизуемся | Что умеет |
|---|---|---|---|
| Партнёрский (tech-partner) | `tech.wazzup24.com/v2` | Basic (email+пароль партнёра) → `machine_token` → `client_access_token` дочки | Только партнёрские методы: ссылки на iframe подключения каналов |
| Рабочий (user API) | `api.wazzup24.com/v3` | `Authorization: Bearer <apiKey>` | Каналы, сообщения, вебхуки, iframe чатов — всё, чем живёт CRM |

Проверка на живых доступах: `client_access_token` дочки (JWT с `child_id`,
`expires_in = 3600`) на `GET https://api.wazzup24.com/v3/channels` отвечает
`401 INVALID_APIKEY — Invalid authorization header`. То же на `/v3/webhooks`.

**Но apiKey у дочернего аккаунта выпустить нельзя — и он не нужен.** White Label
живёт в отдельном контуре целиком: у Wazzup есть полноценный
[Tech Partner API](https://wazzup24.com/help/api/) (`tech.wazzup24.com/v2`) со
своими каналами, сообщениями, вебхуками, пользователями и контактами. Все
вызовы в нём авторизуются тем же `Bearer <client_access_token>`.

Официальных способов получить apiKey ровно три (`/help/api-en/connection-methods/`):
из кабинета аккаунта, WAuth (маркетплейс) и Sidecar (Kommo/Bitrix). Дочерний
WL-аккаунт своего кабинета не имеет — им управляет партнёр по API. Поэтому
«зайти в кабинет дочки и выпустить ключ» — невозможно.

### Два рабочих сценария

**A. CRM остаётся на User API v3 (как сейчас).** Каналы подключаются в кабинете
рабочего аккаунта, `WAZZUP_API_TOKEN` — ключ этого же аккаунта. Кнопка
«Добавить канал» при этом бесполезна, если `WAZZUP_WL_ACCOUNT_ID` указывает на
другой аккаунт: канал уйдёт в дочку и в CRM не появится.

**B. CRM переводится на Tech Partner API v2.** Только так дочерний аккаунт
заработает полностью. Требует второго драйвера в коде — см. ниже.

### Сценарий B: как включить (реализовано)

Драйвер выбирается переменной `WAZZUP_DRIVER` (`wazzup.driver` в yaml):

```
WAZZUP_DRIVER=partner
WAZZUP_WL_BASE_URL=https://tech.wazzup24.com
WAZZUP_WL_EMAIL=<логин партнёра>
WAZZUP_WL_PASSWORD=<пароль партнёра>
WAZZUP_WL_CLIENT_ID=<partner_client_id>
WAZZUP_WL_ACCOUNT_ID=<account_id дочки>
WAZZUP_WL_SCOPE=transport,crm
```

`WAZZUP_API_TOKEN` при `driver=partner` не нужен. Если доступы `wl_*` неполные,
приложение пишет предупреждение в лог и откатывается на `v3` — «тихо сломаться»
не может. На старте драйвер виден в логе:
`[BOOT] Wazzup integration enabled driver=partner base_url=… account_id=…`

Порядок включения:

1. Прописать переменные выше, перезапустить бэкенд.
2. `POST /integrations/wazzup/setup` с `{"webhooks_base_url":"https://<домен>","enabled":true}`
   — подпишет дочку на события. **Это надо сделать до подключения каналов.**
3. Настройки → Каналы мессенджера → «Добавить канал» → подключить каналы.
4. «Обновить» — каналы появятся в списке.

### Различия контуров, которые закрывает драйвер

Текущая интеграция целиком написана под User API v3. Различия, которые
пришлось закрыть:

| Что | User API v3 (сейчас) | Tech Partner API v2 (нужно) |
|---|---|---|
| Авторизация | статический `apiKey` | `client_access_token`, живёт 1 час; `refresh_token` — 7 дней (`client_id` = `WAZZUP_WL_CLIENT_ID`) |
| Каналы | `GET /v3/channels` | `GET /v2/channels`; поля `channel_id`, `messenger_id`, `state`, `status` |
| Отправка | `POST /v3/message` | `POST /v2/messages` |
| Вебхуки | `PATCH /v3/webhooks` — один URI + `subscriptions{}`, авторизация по `crmKey` | `POST /v2/webhooks` — список подписок `{url, event}`, у каждой свой `id` |
| Формат вебхука | `{"messages":[…]}` | `{"event":"message.add","data":[…],"meta":{"idempotency_key":…}}` |
| Дедупликация | `messageId` | `meta.idempotency_key` (таблица `wazzup_dedup` подходит как есть) |

События, на которые надо подписаться: `message.add`, `message.status_update`,
`channel.status_update`, `channel.create`, `channel.qr_update`.

⚠️ **Подписка на вебхуки обязательна ДО подключения каналов** — документация
Wazzup прямо предупреждает: без неё часть каналов не подключается и статусы
не приходят.

### Что НЕ переезжает при смене аккаунта

- **Привязка каналов к филиалам и отделам** (миграции 078, 081) — каналы
  создаются заново с новыми `external_channel_id`, привязки надо проставить
  руками после шага 7.
- **Старые диалоги и переписка** остаются в БД, но отвечать в них уже нельзя:
  `channelId` старого аккаунта в дочке не существует, отправка вернёт ошибку.
  Новые входящие создадут новые диалоги.
- **Лиды и клиенты** остаются на месте — они привязаны к телефону, а не к
  каналу; входящее с того же номера подтянется к существующему лиду
  (см. `processIncomingWebhookMessage`).

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
