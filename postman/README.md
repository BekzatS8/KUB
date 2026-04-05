# Postman: KUB API

## Импорт
1. Импортируйте коллекцию: `postman/KUB API.postman_collection.json`.
2. Импортируйте окружение: `postman/KUB Local.postman_environment.json`.
3. Выберите окружение **KUB Local**.
4. Убедитесь, что `baseUrl = http://localhost:4000`.
5. Заполните локальные credentials (`adminEmail/adminPassword`, `bossEmail/bossPassword`, при необходимости `salesEmail/salesPassword`).

## Структура коллекции
- Health & Bootstrap
- Auth
- Users & Roles
- Clients / Individual
- Clients / Legal
- Leads
- Deals
- Documents / Individual
- Documents / Legal
- Signing
- Chats
- Tasks
- Reports
- Integrations / Wazzup
- Debug / Legacy

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
- legal signer/profile flow:
  - `signerFullName`, `signerPosition`, `signerEmail`, `signerPhone`
  - `legalCompanyName`, `legalBin`, `legalAddress`
  - `contactPersonName`, `contactPersonEmail`, `contactPersonPhone`

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

## Final smoke order для owner (рекомендуемый)
1. **Health & Bootstrap**: `Health Check`, затем `Auth / Login`.
2. **Individual smoke**:
   - `Clients / Individual / Create Client (individual_profile nested)`
   - `Leads / Create Lead`
   - `Leads / Convert Lead To Deal With Client` (или `Deals / Create Deal`)
   - `Documents / Individual / Create Individual Document from client (contract_free_ru)`
3. **Legal smoke**:
   - `Clients / Legal / Create Client (legal_profile nested)`
   - upload corporate files (`Upload Legal File:*`)
   - `Clients / Legal / Get Legal Profile` (проверить `completeness.type=legal`, `missing_contract`, `contract_ready`)
   - создать/выбрать deal
   - `Documents / Legal / Create Legal Document from client (contract_paid_full_ru)`
4. **Signing**:
   - `Signing / Start Signing (Legal representative overrides)` (для legal signer=representative)
   - `Signing Status` -> `Verify Email Token` -> `Confirm Email Token`
   - при необходимости public flow: `Generate Public Sign Link` -> `Public Get Document` -> `Public Sign Document`
5. **Wazzup RBAC check**:
   - `Integrations / Wazzup / Wazzup Setup` под `system_admin` (ожидается success)
   - `Integrations / Wazzup / Wazzup Setup (Leadership should be forbidden)` под `leadership` (ожидается 403)

## Legal client operational flow (recommended)
1. `Clients / Create Client (legal_profile nested)`.
2. `Clients / Get Client Profile` (`GET /clients/:id/profile`) and verify:
   - `completeness.type = legal`
   - `missing_yellow` has only corporate fields.
3. Upload corporate files using `POST /clients/:id/files` with categories:
   - `charter`, `bin_certificate`, `power_of_attorney`, `bank_details`,
   - `director_id`, `representative_id`, `signed_contract`, `corporate_other`.
4. Re-check `GET /clients/:id/profile` readiness.
5. Generate contract via `Documents / Create from client`.
6. Start signing via `Signing / Start Signing (Email)` (optionally with signer override fields).

> `POST /documents/create-from-lead` is legacy and not intended for legal templated contracts.

## Wazzup notes (без секретов в git)
- В Postman **не хранить реальный provider token**.
- `Wazzup Setup` использует серверный config/env (`WAZZUP_ENABLE=true` + token на backend), а не `api_key` в body.
- Для webhook smoke используйте только тестовые значения (`wazzupWebhookToken`, `wazzupVerifyToken`).

## Wazzup
- Запросы в `Integrations/Wazzup` доступны в коллекции даже для локального контура.
- Для работы backend должен быть запущен с `WAZZUP_ENABLE=true`.
- `Wazzup Setup` больше **не требует** `api_key` в body (токен берётся из server config/env).
- `Wazzup Setup` требует JWT пользователя с ролью `system_admin`.

## Debug
- Папка `Debug` работает только в `GIN_MODE != release`.
- Для `/debug/*` нужен JWT пользователя с ролью `system_admin`.
