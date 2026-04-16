# Email signing (ПЭП) flow

SMS variant documentation: `docs/sms_signing.md`.

Дополнительно по встроенному UI: `docs/embedded_signing_ui.md`.

## Environment

Required SMTP and signing configuration:

```bash
export SMTP_HOST="smtp.example.com"
export SMTP_PORT="587"
export SMTP_USER="user"
export SMTP_PASS="pass"
export SMTP_FROM="no-reply@example.com"
export SMTP_FROM_NAME="TurCompany"

# Optional overrides
export SIGN_EMAIL_TTL="30m"                 # default 30 minutes
export SIGN_EMAIL_TOKEN_PEPPER="pepper"     # required in release
export FRONTEND_APP_URL="https://app.example.com"
export PUBLIC_BASE_URL="https://app.example.com"
export SIGN_EMAIL_VERIFY_BASE_URL="https://app.example.com"
```

## Endpoints

### Start signing

```bash
curl -X POST "http://localhost:8080/documents/42/sign/start" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "client@example.com"
  }'
```

### Verify email token (GET only validates)

```bash
curl "http://localhost:8080/api/v1/sign/email/verify?token=TOKEN_FROM_EMAIL&format=json"
```

Response (example):

```json
{
  "token_valid": true,
  "document": {
    "id": 42,
    "title": "contract_paid_50_50_ru",
    "status": "approved",
    "file_name": "contract_paid_50_50_ru.pdf",
    "content_type": "application/pdf",
    "preview_url": "/api/v1/sign/email/preview?token=TOKEN_FROM_EMAIL",
    "document_hash_preview": "sha256:..."
  },
  "agreement": {
    "required": true,
    "version": "v1",
    "title": "Подтверждение ознакомления",
    "checkbox_label": "Я ознакомился с документом, проверил данные и согласен с его условиями.",
    "confirm_button_label": "Перейти к подписанию",
    "version_mismatch_message": "Текст согласия изменился. Пожалуйста, откройте документ заново перед подписанием."
  },
  "confirmation": {
    "expires_at": "2026-04-12T12:00:00Z"
  },
  "require_post_confirm": true
}
```

### Preview document by email token (public, no JWT)

```bash
curl -i "http://localhost:8080/api/v1/sign/email/preview?token=TOKEN_FROM_EMAIL"
```

Endpoint returns current document file `inline` for pre-sign review before `POST /documents/:id/sign/confirm/email`.
Frontend should take `document_hash_preview` from verify response and send it back as `document_hash_from_client` during confirm.
Frontend should also take `agreement.version` from verify response and send it as `agreement_text_version` (do not hardcode agreement text/version in frontend).
Each successful preview call also writes preview audit fields to confirmation meta (`preview_opened_at`, `preview_opened_ip`, `preview_opened_user_agent`, `preview_document_hash`, `preview_file_name`, `preview_content_type`, `preview_open_count`).

### Verify-open vs Preview-open semantics

- `GET /api/v1/sign/email/verify` keeps writing `opened_at/opened_ip/opened_user_agent` as **confirmation-link opened** event.
- `GET /api/v1/sign/email/preview` writes `preview_*` fields as **real document file opened** event.

### Confirm signing (POST)

```bash
curl -X POST "http://localhost:8080/documents/42/sign/confirm/email" \
  -H "Content-Type: application/json" \
  -d '{
    "token": "TOKEN_FROM_EMAIL",
    "code": "123456",
    "agree_terms": true,
    "confirm_document_read": true,
    "agreement_text_version": "v1",
    "document_hash_from_client": "sha256:..."
  }'
```

Response includes sign-session data for finalize step (`session_id`, `session_token`, `sign_url`).
If `agree_terms` or `confirm_document_read` is not `true`, API returns `400 BAD_REQUEST`.
If `agreement_text_version` is missing, API returns `400 BAD_REQUEST`.
If `document_hash_from_client` is missing/invalid, API returns `400 BAD_REQUEST`.
If `agreement_text_version` is outdated and does not match current backend version, API returns `409 CONFLICT` and client should reopen document/verify step.
If document content changed after preview (`document_hash_from_client` != current server hash), API returns `409 CONFLICT` and client should reopen preview.

### Finalize signing session (POST)

```bash
curl -X POST "http://localhost:8080/api/v1/sign/sessions/id/<SESSION_ID>/sign" \
  -H "Content-Type: application/json" \
  -d '{
    "token": "<SESSION_TOKEN>",
    "agree": true
  }'
```

### Status

```bash
curl "http://localhost:8080/documents/42/sign/status" \
  -H "Authorization: Bearer $TOKEN"
```

Response now includes `email_confirmation_audit` with link-open, preview-open, agreement, and hash-verification fields (when present).

## Release note: backend guarantees in this flow

Current backend flow (`verify -> preview -> confirm -> sign session`) guarantees:

1. Verify link opening is tracked (`opened_*` / `link_opened_at`).
2. Real file preview opening is tracked (`preview_*`).
3. Explicit user agreement flags are required (`agree_terms=true`, `confirm_document_read=true`).
4. Agreement version from client must match current backend agreement version.
5. Document hash from client must match current backend document hash.
6. Only after all checks pass does backend approve confirmation and create sign session.

## Testing checklist (local/dev)

1) Apply DB migrations and start the backend.
2) Authorize and create (or pick) a document in `approved` status.
3) Run the signing flow:
   - `POST /documents/:id/sign/start` → expect `{status:"pending", expires_at}`.
   - Open the magic link from email — it should open embedded signing UI page `/sign/email/verify?token=...` (HTML, not raw JSON).
   - Frontend must call `GET /api/v1/sign/email/verify?token=...&format=json` → expect JSON with `require_post_confirm:true`.
   - Frontend may open `preview_url` from verify response to show the file before confirm.
   - `POST /documents/:id/sign/confirm/email` with token+code+agreement flags → expect session payload.
   - `POST /api/v1/sign/sessions/id/:id/sign` with `session_token` → expect `{status:"signed"}`.
   - Repeat `POST /api/v1/sign/sessions/id/:id/sign` with same token → expect stable controlled error (`Session already signed`) without re-sign.
   - `GET /documents/:id/sign/status` → expect `status=signed` and `approved_at`.
4) Negative checks:
   - Wait past TTL and confirm → expect 400 (expired).
   - Reuse the same token after confirm → expect 409 (already used).
   - Expired verify token should return `EXPIRED` and UI should show dedicated expired message.
   - Call `GET /api/v1/sign/email/verify` without token → expect validation error.

## Production checklist (deploy)

1) Apply DB migrations on the server:
   - `db/migrations/003_signature_confirmations.up.sql` must be applied.
2) Configure environment variables for SMTP + signing:
   - `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM`, `SMTP_FROM_NAME`.
   - `SIGN_EMAIL_TOKEN_PEPPER` (required in release).
   - Frontend/public URL vars must be real public domains (not localhost): `FRONTEND_APP_URL`, `PUBLIC_BASE_URL`, `SIGN_EMAIL_VERIFY_BASE_URL`.
   - `SIGN_EMAIL_TTL` (e.g. `30m`).
3) Build and deploy the backend:
   - `go build ./cmd/server` (or your deployment target).
   - Restart the service and verify logs for SMTP + config.
4) Verify end-to-end in production:
   - Use the Postman collection or curl to run Start → Verify (GET) → Confirm (POST) → Status.
