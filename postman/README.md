# Postman: KUB API

## Импорт
1. Импортируйте коллекцию: `postman/KUB API.postman_collection.json`.
2. Импортируйте окружение: `postman/KUB Local.postman_environment.json`.
3. Выберите окружение **KUB Local**.

## Обязательные переменные окружения
- `base_url` — базовый URL API (пример: `http://localhost:8080`)
- `jwt` — access token без префикса `Bearer`
- `client_id` — ID клиента
- `deal_id` — ID сделки
- `doc_id` — заполняется автоматически после генерации документа
- `signed_by` — кто подписал
- `signed_at` — RFC3339 (если оставить пустым, сервер использует текущее время)

> Для совместимости также выставлены алиасы: `baseUrl`, `accessToken`, `clientId`, `dealId`, `documentId`.

## Раздел Documents
Добавлены запросы:
- `GET /documents/types`
- `POST /documents/create-from-client` — 15 запросов, по одному на каждый `doc_type`
- `GET /documents/{{doc_id}}/download?format=pdf`
- `POST /documents/{{doc_id}}/send-for-signature`
- `POST /documents/{{doc_id}}/sign`
- `GET /documents/{{doc_id}}`

У всех create-запросов в тестах сохраняется `doc_id` из ответа (`id` или `data.id`).

## Быстрый сценарий
1. Выполнить `GET document types`.
2. Выполнить любой `Create from client: <doc_type>`.
3. Выполнить `Download PDF by doc_id`.
4. Выполнить `Send for signature`.
5. Выполнить `Sign document`.
6. Проверить статус через `Get document by doc_id`.

## Важно
Если реальные шаблоны еще не загружены в `assets/templates/docx` и `assets/templates/xlsx`, сервер может вернуть `template_not_found` — это ожидаемое поведение.
