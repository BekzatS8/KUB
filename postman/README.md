# KUB Postman коллекция

1) Импортируйте `KUB.postman_collection.json` и `KUB.postman_environment.json`.
   Legacy коллекция `kub_api.postman_collection.json` удалена как устаревшая (в ней нет актуальных маршрутов подписания `/documents/:id/sign/start`, `/documents/:id/sign/status`, `/documents/:id/sign/confirm/email` и `/sign/email/verify`) — используйте только актуальную `KUB.postman_collection.json`.
2) Заполните переменные окружения: `baseUrl`, `email`, `password`, `signerEmail`. `documentId` заполняется автоматически после Create Document (или вручную из List Documents).
3) (Dev) Для debug-эндпоинта задайте `debugKey` и включите `DEBUG_KEY` на сервере; в release debug недоступен.

Последовательность нажатий (без ручных шагов, кроме documentId если не делаете Create):
1) Auth -> Login (в Tests сохранит `accessToken`/`refreshToken`).
2) Documents -> Create Document (в Tests сохранит `documentId`). Если не создаёте документ — выполните Documents -> List Documents и вручную скопируйте `id` в `documentId`.
3) Signing -> Start Signing.
4) Signing -> Signing Status (первичная проверка).
5) Debug -> Latest Sign Tokens (dev only) — заполнит `emailToken`, `tgCallbackToken`.
6) Signing -> Verify Email Token (GET только проверяет токен, без подписи).
7) Signing -> Confirm Email Token (POST подтверждает подпись).
8) Telegram -> Telegram Approve Callback ИЛИ Telegram Reject Callback.
9) Signing -> Signing Status (final) — итоговый статус.

Ожидания по статусам при SIGN_CONFIRM_POLICY:
- ANY: достаточно одного подтверждения (email ИЛИ telegram). После шага 6 или 8 статус должен перейти в подтверждённый.
- BOTH: требуется два подтверждения (email И telegram). После одного канала статус ещё не финальный — нужен второй.

Если Debug endpoint недоступен, заполните вручную переменные окружения:
- `emailToken` — token из magic-link (параметр `token=...` в ссылке).
- `tgCallbackToken` — токен из callback_data в Telegram, формат `sign:approve:<token>` или `sign:reject:<token>`.
