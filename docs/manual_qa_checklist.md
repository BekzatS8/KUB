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

## 1.1) Branches smoke
1. Login as `system_admin`.
2. `GET /branches` — ожидается список из 5 seed-филиалов.
3. `POST /branches` — создать филиал.
4. `PUT /branches/:id` — обновить филиал.
5. `DELETE /branches/:id` — удалить филиал.
6. Login as `leadership`:
   - `GET /branches`, `GET /branches/:id` => 200
   - `POST/PUT/DELETE /branches...` => 403
7. Login as business role (например `sales`):
   - `GET /branches` => 200
   - `GET /branches/:own_branch_id` => 200
   - `GET /branches/:other_id` => 403

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

## 11) Users profile contract (branch-based)
1. `GET /users/me`
2. Проверить shape:
   - `first_name`, `last_name`, `middle_name`, `full_name`
   - `role` (id/code/legacy_name)
   - `branch` (`id,name,code,is_active` или `null`)
   - `telegram`
   - `legacy.company_name`, `legacy.bin_iin`
3. `POST /users` и `PUT /users/:id` (system_admin):
   - передать `branch_id`, `first_name`, `last_name`, `middle_name`, `position`, `is_active`
   - убедиться, что поля сохраняются и возвращаются в enriched profile.

## 12) Branch-scoped business data
1. Создать пользователей в разных филиалах (Branch 1 / Branch 2).
2. Под `sales` Branch 1:
   - создать lead/deal/task/document/chat.
   - убедиться, что в ответах есть `branch_id`.
3. Под `sales` Branch 2:
   - попытка читать/обновлять записи Branch 1 => `403/404` по policy endpoint.
4. Под `operations` Branch 1:
   - видит данные Branch 1, не видит Branch 2.
5. Под `control`/`leadership`/`system_admin`:
   - видят все филиалы;
   - `?branch_id=<id>` ограничивает выборку нужным филиалом.

## 13) Reports by branch
1. Сгенерировать данные минимум в двух филиалах.
2. Проверить:
   - `sales` -> отчёты только по своему филиалу;
   - `operations` -> отчёты своего филиала;
   - `control/leadership/system_admin` -> отчёты по всем филиалам.

## 9) Migrations from zero
1. Start clean DB volume.
2. Run local stack/migrations.
3. Validate tables:
   - `clients`
   - `client_individual_profiles`
   - `client_legal_profiles`
4. Expected: no migration conflicts, app starts.
