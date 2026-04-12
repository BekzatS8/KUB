# Email signing (ПЭП) flow

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
    "status": "approved"
  },
  "confirmation": {
    "expires_at": "2026-04-12T12:00:00Z"
  },
  "require_post_confirm": true
}
```

### Confirm signing (POST)

```bash
curl -X POST "http://localhost:8080/documents/42/sign/confirm/email" \
  -H "Content-Type: application/json" \
  -d '{
    "token": "TOKEN_FROM_EMAIL",
    "code": "123456"
  }'
```

Response includes sign-session data for finalize step (`session_id`, `session_token`, `sign_url`).

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

## Testing checklist (local/dev)

1) Apply DB migrations and start the backend.
2) Authorize and create (or pick) a document in `approved` status.
3) Run the signing flow:
   - `POST /documents/:id/sign/start` → expect `{status:"pending", expires_at}`.
   - Open the magic link from email — it should open embedded signing UI page `/sign/email/verify?token=...` (HTML, not raw JSON).
   - Frontend must call `GET /api/v1/sign/email/verify?token=...&format=json` → expect JSON with `require_post_confirm:true`.
   - `POST /documents/:id/sign/confirm/email` with token+code → expect session payload.
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
