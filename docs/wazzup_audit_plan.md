# Wazzup WhatsApp Integration Audit (Go/Gin + PostgreSQL)

A) КАРТА ПРОЕКТА
1) Список ключевых папок и назначение
- `cmd/web` — точка входа HTTP API (`main -> app.Run`).
- `cmd/bot` — отдельная точка входа бота/интеграционного процесса.
- `internal/app` — bootstrap приложения: загрузка конфига, DB, DI репозиториев/сервисов/хендлеров, middleware, запуск Gin.
- `internal/routes` — централизованная регистрация HTTP маршрутов.
- `internal/handlers` — HTTP-слой (Gin handlers), валидация запроса, маппинг статусов ответов.
- `internal/services` — бизнес-логика (RBAC/правила переходов/оркестрация).
- `internal/repositories` — SQL-доступ к PostgreSQL.
- `internal/models` — DTO/модели API/домена.
- `internal/middleware` — auth/authz/read-only guard.
- `internal/config` — загрузка YAML-конфига + env overrides + валидация.
- `db/migrations` — SQL-миграции схемы.
- `config` — пример/рабочий YAML-конфиг.
- `docs`, `deploy`, `docker-compose*.yml` — деплой и операционные инструкции.

2) Как устроены роуты
- Главная сборка роутов в `internal/routes/routes.go` через `SetupRoutes(...)`.
- Есть публичные пути (`/healthz`, `/auth/*`, `/register*`, webhooks/sign/public docs).
- После `r.Use(authMiddleware)` идут защищённые пути (`/users`, `/clients`, `/leads`, `/deals`, `/documents`, `/chats`, `/tasks`, `/reports`).
- Полноценной общей группы `/api/v1` для всех ресурсов нет.
- Префикс `/api/v1` используется точечно только для sign-сессий (`/api/v1/sign/sessions`).

3) Как устроена конфигурация/секреты
- Конфиг читается из `CONFIG_PATH` либо `config/config.yaml` / `config.yaml`.
- DB DSN: приоритет `DATABASE_URL` env, затем `database.dsn` (или legacy `database.url`/`db.dsn` с нормализацией).
- JWT secret: `security.jwt_secret` либо `JWT_SECRET` env.
- Есть env overrides для SMTP, sign-параметров, Telegram.
- В release-режиме часть секретов/параметров обязательна (`jwt_secret`, SMTP, pepper).
- Сейчас отдельной секции под Wazzup (`api_key`, `crmKey`, base URL) нет.

4) Как устроены миграции
- Миграции — SQL-файлы в `db/migrations`.
- Оркестрация в `docker-compose.prod.yml` через сервис `migrate`, который последовательно выполняет `psql -f ...`.
- Отдельного инструмента типа goose/migrate CLI в коде не найдено; фактически используется raw SQL + psql.

B) ДАННЫЕ (ОЧЕНЬ ВАЖНО)
1) Таблицы и модели
- `leads`:
  - PK: `id SERIAL` (в Go-модели `int`).
  - Поля: `title`, `description`, `owner_id`, `status`, `created_at`.
  - ВАЖНО: `phone` отсутствует.
  - ВАЖНО: `company_id` / `tenant_id` отсутствует.
  - `source` отсутствует (в отчёте подставляется пустая строка).
- `companies/tenants`:
  - Отдельной таблицы `companies` / `tenants` нет.
  - У `users` есть только текстовые `company_name`, `bin_iin` (не FK на tenant-таблицу).
- `users`:
  - PK: `id SERIAL`.
  - Ролевая связь через `role_id`.
  - Привязка к компании — только не-нормализованные поля `company_name/bin_iin`.
- `messages/notes/activities`:
  - Есть `messages`, но это чат-таблица (`chat_id`, `sender_id`, `text`, `attachments`) для внутреннего чата.
  - Есть `tasks` и `audit_logs`; полноценной CRM timeline по лидам/клиентам (универсальной activity feed) нет.

2) Если нет `phone` у lead/contact
- У `clients` поле `phone` уже есть.
- У `leads` `phone` нет -> минимально безопасно добавить:
  - `ALTER TABLE leads ADD COLUMN phone VARCHAR(50);`
  - индекс `CREATE INDEX ... ON leads(phone)` (non-unique).
- Опционально и полезно для аналитики/inbound:
  - `source VARCHAR(50)` с default `'manual'` или nullable + app-level нормализация.
  - `external_channel VARCHAR(30)` / `external_chat_id` только если действительно нужно связывать чат с лидом без новой таблицы.

3) Где хранить incoming WhatsApp messages
- Не использовать текущую `messages` как есть: она жёстко завязана на `chat_id` внутреннего чата и `sender_id` пользователя CRM.
- Предпочтительно:
  - Если нужна только фиксация первого сообщения для создания лида — сохранить текст в `leads.description` (+ `phone`, `source='whatsapp'`) и добавить событие в `audit_logs.meta`.
  - Если нужен полноценный журнал входящих сообщений — лучше отдельная таблица интеграции (минимальная), но только после подтверждения требований по истории переписки.
- Для текущей цели (не терять первый входящий лид) минимальный путь — без новой таблицы сообщений.

C) АУТЕНТИФИКАЦИЯ И ПРАВА
1) Как определяется company_id
- Сейчас `company_id` не определяется: в JWT claims только `user_id` и `role_id`.
- Middleware кладёт в context именно `user_id`, `role_id`.
- Текущая модель доступа фактически user/role-based, а не tenant-based.

2) Как безопасно привязать webhook к company
Варианты:
- Вариант A: `POST /integrations/wazzup/webhook/:company_id`.
  - Минус: company_id легко перечисляемый, если нет доп. секрета.
- Вариант B: единый webhook + проверка per-company секрета (`crmKey`) из заголовка/поля payload.
  - Плюс: лучше для multi-tenant и безопасности.
- Вариант C (рекомендуемый в текущей архитектуре):
  - `POST /integrations/wazzup/webhook/:integration_id` (непрозрачный id/slug) + валидация `crmKey`.
  - Маппинг `integration_id -> owner/company scope` в БД.

3) Что лучше для этой архитектуры
- С учётом отсутствия нормального tenant FK, best-effort подход:
  1) Ввести сущность/конфиг интеграции на уровень «компания пользователя» (пока через owner/admin user scope).
  2) Всегда валидировать `crmKey` (секрет per integration).
  3) Для создания лида назначать `owner_id` детерминированно (например, default manager/creator integration).
- Без этого вебхук нельзя надёжно/безопасно маршрутизировать между арендаторами.

D) WAZZUP ИНТЕГРАЦИЯ — ТОЧКИ ВСТРАИВАНИЯ
1) Где разместить код
- Рекомендуемая структура:
  - `internal/integrations/wazzup/client.go` — HTTP клиент Wazzup API.
  - `internal/integrations/wazzup/service.go` — use-cases (iframe URL, webhook flow, dedup).
  - `internal/repositories/wazzup_repository.go` — SQL для конфигов интеграции/дедуп-меток (если понадобится).
  - `internal/handlers/wazzup_handler.go` — HTTP endpoints.
- Аргумент: в проекте уже есть паттерн handler/service/repository, плюс integrations handler для Telegram — можно переиспользовать стиль, но не перегружать один файл.

2) Минимальные endpoints
- `POST /api/v1/integrations/wazzup/webhook` (public, без JWT, с проверкой подписи/`crmKey`).
- `POST /api/v1/integrations/wazzup/iframe` (JWT, получить URL iFrame для номера/лида).
- Опционально `POST /api/v1/integrations/wazzup/setup` (JWT admin/mgmt): сохранить webhooksUri/crmKey/api key reference.

Примечание по текущему проекту:
- Сейчас API в основном без общего `/api/v1`; если важно единообразие, можно:
  - либо добавить новые пути в стиле текущего проекта (`/integrations/wazzup/*`),
  - либо начать аккуратный переход к `/api/v1/...` только для новых интеграций.

3) Поток данных
- Webhook:
  1) Приём payload.
  2) Верификация `crmKey` + валидация обязательных полей.
  3) Dedup по `message_id`/`event_id`.
  4) По нормализованному телефону ищем сущность:
     - сначала `client` (если бизнесу так нужно),
     - иначе `lead` по телефону (после добавления поля),
     - иначе создаём `lead` со статусом `new`, source=`whatsapp`.
  5) Сохраняем первый текст (минимум в `lead.description`/audit).
  6) Возвращаем 200, чтобы Wazzup не ретраил бесконечно.
- Iframe:
  1) JWT запрос из CRM карточки.
  2) Берём телефон из lead/client.
  3) Вызываем Wazzup API для chat iframe URL.
  4) Возвращаем URL фронту.

4) Dedup в рамках текущей БД
- Лучший минимальный вариант без новой таблицы на старте:
  - писать в `audit_logs` событие вида `wazzup.webhook.received` и сохранять `event_id` в `meta`.
  - перед обработкой проверять существование `event_id` в `audit_logs.meta` (индексировать expression при росте нагрузки).
- Риск: `audit_logs` изначально не оптимизирован под high-throughput dedup, но для MVP может хватить.
- Более надёжно — отдельная таблица идемпотентности (если ожидается высокий поток).

E) РЕЗУЛЬТАТ
1) Список необходимых изменений (план, без кода)
1.1. Data model (минимум)
- Добавить в `leads` поле `phone` (+ индекс).
- Добавить в `leads` поле `source` (или договориться хранить source в description/meta; предпочтительно отдельное поле).
- (Опц.) таблица конфигурации Wazzup-интеграции per tenant/company-scope.

1.2. Configuration & secrets
- Добавить config/env для Wazzup base URL и API key (dev/prod разделение).
- Запретить hardcode ключей в коде; тестовый ключ только через `.env.dev`/локальный config.

1.3. Application layer
- Добавить Wazzup client/service/handler.
- Подключить роуты public webhook + private iframe/setup.
- Добавить RBAC на setup/iframe.

1.4. Webhook flow
- Валидация подписи (`crmKey`).
- Нормализация телефона.
- Idempotency check (через audit_logs или отдельное хранилище).
- find/create lead + сохранение first message.

1.5. UI contract (backend API)
- Endpoint для получения iframe URL по `lead_id`/`client_id`.
- Стандартизированный ответ: `{url, expires_at?, chat_id?}`.

2) Риски
- Критичный архитектурный риск: в текущей схеме нет `company_id/tenant_id` и нет таблицы компаний -> multi-tenant привязка будет эвристикой через owner/user scope.
- Риск коллизий по телефону (один номер у разных «компаний»).
- Риск неправильной идемпотентности, если dedup делать только через `audit_logs` без уникального ограничения.
- Риск несовместимости, если начать использовать `/api/v1` точечно при текущем смешанном роутинге.
- Риск утечки секретов при хранении api key/crmKey в YAML/Git.

3) Промт №2 (для этапа реализации data layer)
""
Ты senior Go backend engineer. На основе фактической схемы KUB CRM реализуй data layer для Wazzup-интеграции.

Требования:
1) Сначала используй текущие типы ключей из проекта (SERIAL/int), не переходи на UUID.
2) Сделай минимальные миграции:
   - leads.phone (если отсутствует) + индекс;
   - leads.source (если отсутствует).
   - Если обоснуешь необходимость, добавь минимальную таблицу `wazzup_integrations` (id SERIAL, owner_user_id INT FK users, crm_key_hash TEXT, api_key_encrypted TEXT/или external ref, webhook_token TEXT UNIQUE, created_at).
3) Реализуй репозитории:
   - FindLeadByPhoneWithinScope(...)
   - CreateLeadFromInbound(...)
   - SaveDedupEvent(...)
   - IsDuplicateEvent(...)
4) Для dedup сначала используй `audit_logs` (action + meta.event_id), но с возможностью переключить на отдельную таблицу.
5) Покрой unit-тестами репозитории (sqlmock либо test DB).
6) Не пиши HTTP handlers на этом шаге — только миграции + repository/service data-layer contracts.

Отдай:
- SQL миграции up/down,
- интерфейсы и реализации репозиториев,
- тесты,
- короткий ADR (почему так, и где ограничение multi-tenant в текущей схеме).
""
