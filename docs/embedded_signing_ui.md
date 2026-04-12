# Embedded public signing UI

This project includes a built-in public HTML signing UI served by backend.

## Public routes

- `GET /sign/email/verify?token=...` — browser entrypoint, renders embedded signing page.
- `GET /api/v1/sign/email/verify?token=...&format=json` — API verify endpoint used by the page (and future external frontend).
- `POST /documents/:id/sign/confirm/email` — confirm token + OTP code, creates sign session.
- `POST /api/v1/sign/sessions/id/:id/sign` — finalize signing with session token.

## End-to-end user flow

1. User gets email with signing link.
2. User opens `/sign/email/verify?token=...`.
3. Embedded UI calls `/api/v1/sign/email/verify?...&format=json`.
4. User enters OTP code and submits to `/documents/:id/sign/confirm/email`.
5. UI receives `session_id` + `session_token` and calls `/api/v1/sign/sessions/id/:id/sign`.
6. UI shows success state after final sign.

## Error/edge cases

- Expired token is shown as a dedicated message: "Ссылка истекла. Попросите отправить новую ссылку."
- Invalid token is shown as a separate message ("Ссылка недействительна..."), not mixed with expired.
- Repeated finalize call is stable: backend returns controlled `already signed` error and does not re-run signing business logic.
- Signing page always shows document metadata (`title`, `id`, `status`). In this embedded mode file preview is not exposed publicly.

## Production URL rules

In `release` mode the following values must be absolute non-localhost URLs:

- `frontend.host`
- `public_base_url`
- `sign_base_url`
- `sign_email_verify_base_url`

## Manual check

1. Start signing (`POST /documents/:id/sign/start`).
2. Open link from email in browser.
3. Ensure HTML page is shown (not JSON).
4. Enter OTP and confirm.
5. Click sign button.
6. Verify document status changed to `signed`.
