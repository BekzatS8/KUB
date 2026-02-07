# Email signing (ПЭП) flow

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
curl "http://localhost:8080/sign/email/verify?token=TOKEN_FROM_EMAIL"
```

### Confirm signing (POST)

```bash
curl -X POST "http://localhost:8080/documents/42/sign/confirm/email" \
  -H "Content-Type: application/json" \
  -d '{
    "token": "TOKEN_FROM_EMAIL"
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
   - Open the magic link and call `GET /sign/email/verify?token=...` → expect JSON with `require_post_confirm:true`.
   - `POST /documents/:id/sign/confirm/email` with the token → expect `{status:"signed"}`.
   - `GET /documents/:id/sign/status` → expect `status=signed` and `approved_at`.
4) Negative checks:
   - Wait past TTL and confirm → expect 400 (expired).
   - Reuse the same token after confirm → expect 409 (already used).
   - Call `GET /sign/email/verify` → status should remain unsigned.

## Production checklist (deploy)

1) Apply DB migrations on the server:
   - `db/migrations/003_signature_confirmations.up.sql` must be applied.
2) Configure environment variables for SMTP + signing:
   - `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM`, `SMTP_FROM_NAME`.
   - `SIGN_EMAIL_TOKEN_PEPPER` (required in release).
   - `SIGN_EMAIL_TTL` (e.g. `30m`) and `SIGN_EMAIL_VERIFY_BASE_URL` (public URL).
3) Build and deploy the backend:
   - `go build ./cmd/server` (or your deployment target).
   - Restart the service and verify logs for SMTP + config.
4) Verify end-to-end in production:
   - Use the Postman collection or curl to run Start → Verify (GET) → Confirm (POST) → Status.
