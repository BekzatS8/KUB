# Postman: KUB API

## Импорт
1. Импортируйте коллекцию: `postman/KUB API.postman_collection.json`.
2. Импортируйте окружение: `postman/KUB Local.postman_environment.json`.
3. Выберите окружение **KUB Local**.
4. Убедитесь, что `baseUrl = http://localhost:4000`.

## Структура коллекции
- Auth
- Users & Roles
- Clients
- Leads
- Deals
- Documents
- Signing
- Chats
- Tasks
- Reports
- Integrations/Wazzup
- Debug (dev-only)

## Ключевые переменные environment
- `baseUrl`, `accessToken`, `refreshToken`
- `adminEmail`, `adminPassword`, `bossEmail`, `bossPassword`
- `userId`, `roleId`
- `clientId`, `individualClientId`, `legalClientId`
- `leadId`, `dealId`, `documentId`
- `chatId`, `messageId`
- `signSessionId`, `signSessionToken`, `publicDocumentToken`
- `wazzupVerifyToken`, `wazzupChatId`, `wazzupWebhookToken`
- `emailVerificationCode`, `registerUserId`

> Для обратной совместимости сохранены алиасы (`base_url`, `jwt`, `client_id`, `lead_id`, `deal_id`, `doc_id`, `chat_id`, `message_id`, `sign_session_id`, `session_token`).

## Полезные автоскрипты (Tests)
- После `Auth / Login` сохраняются `accessToken` и `refreshToken`.
- После `Clients / Create...` сохраняется `clientId`.
- После `Leads / Create Lead` сохраняется `leadId`.
- После `Leads / Convert...` сохраняется `dealId`.
- После `Documents / Create...` сохраняется `documentId`.
- После `Chats / Create...` и `Chats / Send Message` сохраняются `chatId`/`messageId`.
- После `Signing / Confirm Email Token` сохраняются `signSessionId` и `signSessionToken`.

## Быстрый smoke flow (локально)
1. `Auth / Login`
2. `Clients / Create Client (individual_profile nested)` или `... (legal_profile nested)`
3. `Leads / Create Lead`
4. `Leads / Convert Lead To Deal` (или `...With Client`)
5. `Documents / Create from client: <doc_type>`
6. `Signing / Start Signing (Email)` → `Verify Email Token` → `Confirm Email Token` → `Signing Status`
7. `Chats / Create Personal Chat` → `Send Message`
8. `Reports / Funnel|Leads|Revenue`

## Wazzup
- Запросы в `Integrations/Wazzup` доступны в коллекции даже для локального контура.
- Для работы backend должен быть запущен с `WAZZUP_ENABLE=true`.
- `Wazzup Setup` больше **не требует** `api_key` в body (токен берётся из server config/env).
