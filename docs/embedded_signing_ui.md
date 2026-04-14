# Embedded public signing UI

This project includes a built-in public HTML signing UI served by backend.

## Public routes

- `GET /sign/email/verify?token=...` — browser entrypoint, renders embedded signing page.
- `GET /api/v1/sign/email/verify?token=...&format=json` — API verify endpoint used by the page (and future external frontend).
- `GET /api/v1/sign/email/preview?token=...` — public preview endpoint for inline document view before confirmation.
- `POST /documents/:id/sign/confirm/email` — confirm token + OTP code + agreement flags (`agree_terms`, `confirm_document_read`, `agreement_text_version`), creates sign session.
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
- Verify API now also returns `preview_url` and `document_hash_preview` so UI can show exact server-side file before confirm.
- Verify API also returns backend-owned `agreement` block (version/text labels). Frontend should use this payload and send `agreement.version` back as `agreement_text_version` on confirm.
- Confirm request must include `document_hash_from_client` (value from `document_hash_preview`); if hash mismatches, backend returns `409` and user should reopen document preview.
- If `agreement_text_version` no longer matches backend current version, confirm returns `409` and user should reopen verify/preview flow.
- `verify` (`opened_*`) and `preview` (`preview_*`) are tracked separately in confirmation meta: link-open vs real file-open events.
- Internal `GET /documents/:id/sign/status` now includes `email_confirmation_audit` block with these audit fields for backend/admin visibility.

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
