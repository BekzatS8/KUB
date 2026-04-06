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

## 7) Chat smoke
1. Create group chat.
2. Send message.
3. List messages.
4. Expected: message has sender + sender profile payload.

## 8) Wazzup smoke
1. Enable Wazzup env config (`WAZZUP_ENABLE=true`, token via `.env.local`).
2. `POST /integrations/wazzup/setup` (JWT, `system_admin`).
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
