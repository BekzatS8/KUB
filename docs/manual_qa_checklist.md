# Manual QA smoke checklist (post-integration hardening)

## 0) Environment boot
1. `cp .env.local.example .env.local`
2. `cp config/config.local.example.yaml config/config.local.yaml`
3. `make local-up`
4. Check health: `curl -fsS http://localhost:4000/healthz`

## 1) Registration -> confirm -> login
1. `POST /register`
2. Read verify code from API logs (`[DEV][email][verify]`).
3. `POST /register/confirm`
4. `POST /auth/login`
5. Expected: access+refresh tokens, 200.

## 2) Prepare role for smoke
- Promote first user to leadership (`role_id=40`) for business smoke, or system_admin (`role_id=50`) for system/integrations checks (dev only).

## 3) Create individual client
1. `POST /clients` with `client_type=individual`, `first_name`, `last_name`, `phone`, `birth_date`, `country`, `trip_purpose`.
2. `GET /clients/:id`
3. Expected: base fields + `individual_profile`, status 201/200.

## 4) Create legal client
1. `POST /clients` with `client_type=legal` and either:
   - flat fields (`name`, `bin_iin`, `phone`, `address`) or
   - nested `legal_profile` (`company_name`, `bin`, `contact_person_name`, `contact_person_phone`, `legal_address`).
2. `GET /clients/company`
3. Expected: legal client appears with correct `display_name` and legal profile.

## 5) Lead + deal with client
1. `POST /leads`
2. Convert lead to deal with created typed ref (`client_id` + `client_type`).
3. Проверить негатив: wrong `client_type` для того же `client_id` => 4xx.
4. Expected: deal created and references existing `clients.id` с корректным `client_type` в ответе сделки.

## 6) Document generation + signing
1. `POST /documents/create-from-client` с typed ref (`client_id` + `client_type`).
2. Проверить негатив: без `client_type` => fail, с wrong type => fail.
3. Проверить `deal_id=0`: берётся последняя сделка именно по typed ref.
4. Start sign session endpoint.
5. Complete sign flow (`verify` + `sign`).
6. Expected: signed state transition is successful.

## 7) Chat smoke (role-aware)
### 7.1 Sales
1. Login as `sales`.
2. `GET /chats/users?query=` — directory должен вернуть safe-lite users + `existing_personal_chat_id`.
3. `POST /chats/personal` (target из directory).
4. `POST /chats/:id/messages`.
5. `GET /chats`, `GET /chats/search`, `GET /chats/:id/info`.
6. Expected:
   - personal chat содержит `counterparty`,
   - group chat (если есть) содержит `participants_preview`,
   - messages содержат `sender_profile`.

### 7.2 Operations
1. Login as `operations`.
2. Повторить шаги sales.
3. Дополнительно проверить поиск `GET /chats/search` по display_name/email собеседника (personal chat с пустым `name` должен находиться).

### 7.3 Control (read-only)
1. Login as `control`.
2. Проверить read-path: `GET /chats/users`, `GET /chats`, `GET /chats/:id/info`.
3. Проверить текущую policy chat-write:
   - `POST /chats/personal`
   - `POST /chats/:id/messages`
   - `POST /chats/:id/read`
4. Ожидание: chat-write разрешён по узкому исключению, но write по бизнес-эндпоинтам (`POST /clients`, `POST /deals` и т.п.) остаётся `403`.

## 8) Wazzup smoke
1. Enable Wazzup env config (`WAZZUP_ENABLE=true`, token via `.env.local`).
2. `POST /integrations/wazzup/setup` (JWT, any authenticated known role).
3. `POST /integrations/wazzup/iframe` с пустым payload `{}` (global iframe, без `lead_id/client_id`).
4. `POST /integrations/wazzup/send` with `chat_id` + `text`.
5. Expected: setup/send return 200 or controlled 502 on provider issues, app stays healthy.

## 10) Debug endpoint guard (dev-only)
1. Run app in `GIN_MODE=debug`.
2. `GET /debug/register-verification/latest?user_id=<id>` and `GET /debug/sign-confirmations/latest?document_id=<id>` with JWT `system_admin`.
3. Repeat call with JWT role `leadership`.
4. Expected: `system_admin` gets 200/404 by data availability; non-system-admin gets 403.

## 9) Migrations from zero
1. Start clean DB volume.
2. Run local stack/migrations.
3. Validate tables:
   - `clients`
   - `client_individual_profiles`
   - `client_legal_profiles`
4. Expected: no migration conflicts, app starts.

## 11) Multi-company strictness (JWT + reports + companies ACL)
1. Login as a user that has access to at least 2 companies.
2. Verify `POST /auth/login` response returns `active_company_id`.
3. Decode returned access token and verify claim `active_company_id` is present.
4. Call `POST /auth/select-company` with another company id.
5. Expected:
   - response contains `active_company_id`,
   - response returns a **new** `access_token`,
   - new token contains updated `active_company_id`.
6. Call `/reports/funnel`, `/reports/leads`, `/reports/revenue`:
   - without active company context => 400,
   - with active company context => 200 and company-scoped data.
7. Call `/companies` and `/companies/:id`:
   - any authenticated user gets membership-scoped list on `/companies`,
   - `/companies/:id` returns `200` for membership company and `404` otherwise.
8. Call `GET /users/me/companies` and verify response includes:
   - `companies`
   - `primary_company_id`
   - `active_company_id`
9. Switch company via `PATCH /users/me/active-company` and verify response includes:
   - `active_company_id`
   - fresh `access_token` (`tokens.access_token` alias also present).
10. Verify `/companies` ACL:
   - list returns only membership companies for current user.
11. Verify `/companies/:id` ACL:
   - existing membership => 200
   - no membership => 404.
12. Verify `/companies/:id/integrations` ACL:
   - `leadership/system_admin` + membership => allowed,
   - other roles => 403,
   - leadership/system_admin without membership => denied.

## 12) Repository SQL consistency (company_id wiring)
1. Create lead/deal/document/task in active company A.
2. Switch active company to B and create the same entity types again.
3. Verify list responses for each module do not mix `company_id` values and every item has expected `company_id`:
   - `/leads`, `/deals`, `/documents`, `/tasks`, `/chats`.
4. Update a deal and a document, then re-read:
   - ensure no SQL/scan errors,
   - ensure `company_id` remains stable (cannot be tampered via payload).
5. Convert lead to deal:
   - ensure created deal inherits lead company (`company_id`),
   - ensure no insert/scan mismatch errors.
