# SMS signing via Mobizon

SMS flow mirrors email flow and reuses the same embedded signing UI pattern.

## Routes

- `POST /documents/:id/sign/start/sms`
- `GET /sign/sms/verify?token=...`
- `GET /api/v1/sign/sms/verify?token=...&format=json`
- `GET /api/v1/sign/sms/preview?token=...`
- `POST /documents/:id/sign/confirm/sms`
- Finalize (shared): `POST /api/v1/sign/sessions/id/:id/sign`

## Required env

- `MOBIZON_ENABLED`
- `MOBIZON_API_KEY`
- `MOBIZON_BASE_URL`
- `MOBIZON_FROM`
- `MOBIZON_TIMEOUT_SECONDS`
- `MOBIZON_RETRIES`
- `MOBIZON_DRY_RUN`
- `SIGN_SMS_VERIFY_BASE_URL`
- `SIGN_SMS_TTL`

## QA

1. Start SMS signing with `POST /documents/:id/sign/start/sms`.
2. Ensure recipient gets OTP + link SMS.
3. Open `/sign/sms/verify?token=...` and confirm preview/consent + OTP.
4. Verify response returns sign session (`session_id`, `session_token`, `sign_url`).
5. Complete final sign using shared sign-session endpoint.
6. Check `/documents/:id/sign/status` includes `sms_confirmation_audit`.
