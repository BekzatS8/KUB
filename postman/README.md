# KUB Postman коллекция

1) Импортируйте `KUB.postman_collection.json` и `KUB.postman_environment.json`.
2) Заполните базовые переменные окружения:
   - `baseUrl`
   - `email`, `password` (для логина)
   - `companyName`, `phone` (для регистрации)
3) Остальные переменные (`userId`, `clientId`, `leadId`, `dealId`, `documentId`, `taskId`, `chatId`) заполняются автоматически тест-скриптами после `Create *` запросов. При необходимости можно указать вручную.
4) (Dev) Debug-эндпоинты доступны только вне `GIN_MODE=release`.

Последовательность нажатий (без ручных шагов, кроме documentId если не делаете Create):
1) Auth -> Login (в Tests сохранит `accessToken`/`refreshToken`).
2) Documents -> Create Document (в Tests сохранит `documentId`). Если не создаёте документ — выполните Documents -> List Documents и вручную скопируйте `id` в `documentId`.
3) Signing -> Start Signing.
4) Signing -> Signing Status (первичная проверка).
5) Debug -> Latest Sign Tokens (dev only) — заполнит `email_token`, `tgCallbackToken`.
6) Signing -> Verify Email Token (GET только проверяет токен, без подписи).
7) Signing -> Confirm Email Token (POST подтверждает подпись).
8) Signing -> Serve Sign Page (использует `sign_session_id` + `session_token`).
9) Signing -> Sign Session.
10) Signing -> Signing Status (final) — итоговый статус.

Ожидания по статусам при SIGN_CONFIRM_POLICY:
- ANY: после email-confirm создаётся sign session; финальный signed после шага Sign Session.
- BOTH: требуется два подтверждения (email И telegram). После одного канала статус ещё не финальный — нужен второй.

Если Debug endpoint недоступен, заполните вручную переменные окружения:
- `email_token` — token из magic-link (параметр `token=...` в ссылке).
- `otp_code` — шестизначный код из письма.
- `session_token` и `sign_session_id` — берутся из ответа `Confirm Email Token`.

## Быстрый старт для фронтендера

1) Register -> Register (сохранит `userId`), затем Register -> Register Confirm.
2) Auth -> Login (сохранит `accessToken`/`refreshToken`).
3) Используйте разделы Users/Clients/Leads/Deals/Documents/Tasks/Chats/Reports для проверки всего API.
4) Для файловых ручек (`documents/upload`, `chats/{id}/upload`) заполните `uploadFile`.
