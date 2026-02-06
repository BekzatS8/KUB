# KUB Postman коллекция

1) Импортируйте `KUB.postman_collection.json` и `KUB.postman_environment.json`.
2) Заполните переменные окружения: `baseUrl`, `email`, `password`, при необходимости `documentId`.
3) (Dev) Для debug-эндпоинта задайте `debugKey` и включите `DEBUG_KEY` на сервере; в release debug недоступен.

Smoke Run:
1) Health -> Login -> (Create Document или List/Get Document) -> Start Signing -> Signing Status.
2) Получите `emailCode`/`emailToken`/`tgCallbackToken` вручную или через Debug (dev only).
3) Confirm Email Code -> Verify Email Token -> Telegram Approve/Reject -> Signing Status.
4) Для проверки политики ANY/BOTH поменяйте `signPolicy` и повторите Start Signing.
