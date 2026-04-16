# KUB API — CRM-бэкенд с RBAC, верификацией телефона и документооборотом

KUB — это REST API на **Go + Gin**, с ролевой моделью доступа (**RBAC**), безопасной регистрацией через **код подтверждения**, управлением лидами/сделками/задачами/документами, и отчётами.  
Хранилище — **PostgreSQL**. PDF-генерация — встроенная.

---

## Содержание

- [Возможности](#возможности)  
- [Архитектура и стек](#архитектура-и-стек)  
- [Быстрый старт](#быстрый-старт)  
- [Конфиг](#конфиг)  
- [Миграции БД](#миграции-бд)  
- [Аутентификация и безопасность](#аутентификация-и-безопасность)  
- [RBAC (роли и права)](#rbac-роли-и-права)  
- [RBAC policy model](docs/rbac.md)
- [Wazzup / WhatsApp integration](docs/integrations/wazzup.md)
- [Clients data model](docs/clients.md)
- [Manual QA smoke checklist](docs/manual_qa_checklist.md)
- [Smoke map (E2E)](docs/smoke_map.md)
- [Эндпоинты](#эндпоинты)  
- [Коллекция Postman](#коллекция-postman)  
- [Требования/запуск в проде](#требованиязапуск-в-проде)
- [Деплой через Docker Compose](#деплой-через-docker-compose)
- [Деплой через systemd (без Docker)](#деплой-через-systemd-без-docker)
- [Проверка генерации документов](#проверка-генерации-документов)
- [Логи и конфигурация](#логи-и-конфигурация)
- [Траблшутинг](#траблшутинг)
- [Структура репо](#структура-репо)  
- [Примеры запросов (быстрый сценарий)](#примеры-запросов-быстрый-сценарий)

---

## Возможности

- ✅ Single-company CRM: одна компания, 5 филиалов (`branches`), branch-based data scope
- ✅ Регистрация пользователя с верификацией телефона
- ✅ Троттлинг и защита от брутфорса для кода подтверждения
- ✅ Безопасное хранение кода (bcrypt-хэш), TTL, попытки, resend-лимиты
- ✅ JWT-аутентификация + ротация refresh-токена
- ✅ RBAC: sales / operations / control / leadership / system_admin
- ✅ Лиды, сделки, задачи, сообщения; документы с подтверждением подписи кодом
- ✅ Отчёты и фильтры
- ✅ Скачивание/просмотр PDF из защищённого стора

---

## Архитектура и стек

- **Go 1.22+**, **Gin** (HTTP, middleware)
- **PostgreSQL** (модель данных и индексы заточены под RBAC/аудит/фильтры)
- **SMTP** (почта)
- **JWT**: access (короткий), refresh (длинный, хранится в БД в hashed-виде с ротацией)
- **bcrypt** для паролей и верификационных кодов
- Генерация PDF (пользовательские шаблоны/шрифты)

---

## Быстрый старт

1) Установи Go и PostgreSQL.  
2) Создай БД и накатай миграции (см. ниже).  
3) Скопируй пример и заполни `config/config.yaml`.  
4) Запуск:
```bash
go run cmd/web/main.go
# или
GIN_MODE=release go run cmd/web/main.go
```
Сервер поднимется на `http://localhost:<port>` (по умолчанию 4000).

---

## Локальный запуск после чистого клона

> Цель local-режима: быстро поднять `postgres + migrate + api` в Docker без реальных SMTP/Telegram/Wazzup секретов.

### 1) Подготовка локальных файлов

**Bash (Linux/macOS/Git Bash):**
```bash
cp .env.local.example .env.local
cp config/config.local.example.yaml config/config.local.yaml
mkdir -p files/pdf files/docx files/excel
```

**PowerShell (Windows):**
```powershell
Copy-Item .env.local.example .env.local -ErrorAction SilentlyContinue
Copy-Item config/config.local.example.yaml config/config.local.yaml -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force files,files/pdf,files/docx,files/excel | Out-Null
```

### 2) Старт локального стека

**Bash:**
```bash
make local-up
```

**PowerShell:**
```powershell
docker compose -f docker-compose.local.yml up -d --build
```

Проверки:
```bash
docker compose -f docker-compose.local.yml ps
curl -fsS http://localhost:4000/healthz
```

### 3) Логи, остановка, миграции, psql

```bash
make local-logs      # хвост логов
make local-migrate   # ручной прогон миграций
make local-psql      # psql в контейнер postgres
make local-down      # остановка локального стека
```

### 4) Как создать первого пользователя в debug-режиме

1. Зарегистрируй пользователя публичным endpoint:
```bash
curl -X POST http://localhost:4000/register \
  -H "Content-Type: application/json" \
  -d '{"first_name":"Dev","last_name":"User","branch_id":1,"company_name":"Dev Co","bin_iin":"123456789012","email":"first@local.dev","password":"Passw0rd!","phone":"+77000000000"}'
```

2. Получи verification code из логов API (dev-лог):
```bash
docker compose -f docker-compose.local.yml logs -f api | rg "\[DEV\]\[email\]\[verify\]"
```

3. Подтверди регистрацию:
```bash
curl -X POST http://localhost:4000/register/confirm \
  -H "Content-Type: application/json" \
  -d '{"user_id":1,"code":"<CODE_FROM_LOGS>"}'
```

### 5) Временное повышение первого пользователя до leadership (role_id=40)

Для локального smoke чаще всего нужен полный доступ к бизнес-сущностям (`leadership`, role_id=40), поэтому можно временно повысить первого пользователя SQL-командой:

```sql
UPDATE users SET role_id = 40 WHERE id = 1;
```

Команда из хоста:
```bash
docker compose -f docker-compose.local.yml exec -T postgres \
  psql -U "${POSTGRES_USER:-turcompany}" -d "${POSTGRES_DB:-turcompany}" \
  -c "UPDATE users SET role_id = 40 WHERE id = 1;"
```

> Не коммить `config/config.local.yaml`, `.env.local` и любые реальные секреты.

## Конфиг

Файл: `config/config.yaml` (скопируй из `config/config.example.yaml`)
```yaml
server:
  port: 4000

database:
  url: "postgres://user:pass@localhost:5432/kub?sslmode=disable"

email:
  smtp_host: "smtp.mail.ru"
  smtp_port: 465
  smtp_user: "noreply@yourdomain.kz"
  smtp_password: "APP_PASSWORD"    # пароль приложения (см. Траблшутинг)
  from_email: "noreply@yourdomain.kz"

files:
  root_dir: "./files"               # корень хранилища документов

sign_base_url: "https://YOUR-DOMAIN.TLD/sign"
sign_confirm_policy: "ANY"         # ANY или BOTH (Email + Telegram)
sign_email_verify_base_url: "https://YOUR-DOMAIN.TLD"
sign_sms_verify_base_url: "https://YOUR-DOMAIN.TLD"
sign_email_ttl_minutes: 30
sign_sms_ttl_minutes: 30
sign_session_ttl_minutes: 30   # если не задано/<=0 -> берётся sign_email_ttl_minutes
mobizon:
  enabled: false
  api_key: ""
  base_url: "https://api.mobizon.kz"
  from: ""
  timeout_seconds: 10
  retries: 1
  dry_run: true

security:
  jwt_secret: "CHANGE_ME"

cors:
  allow_origins: "*"
  allow_methods: "GET, POST, PUT, DELETE, OPTIONS"
  allow_headers: "Origin, Content-Type, Authorization"
  expose_headers: "Content-Disposition, Content-Type, Content-Length"
```

### Подписание документа (Email + Telegram)

Подпись подтверждается через email и telegram. Политика задаётся параметром `sign_confirm_policy`: `ANY` (достаточно одного канала) или `BOTH` (нужны оба канала). 

```bash
# базовый URL страницы подписи (без токена)
SIGN_BASE_URL="https://example.com/sign"

# политика подтверждения подписи: ANY (email или telegram) / BOTH (email и telegram)
SIGN_CONFIRM_POLICY="ANY"

# базовый URL для email-подтверждения подписи
SIGN_EMAIL_VERIFY_BASE_URL="https://example.com"
SIGN_SMS_VERIFY_BASE_URL="https://example.com"
MOBIZON_ENABLED="false"
MOBIZON_API_KEY=""
MOBIZON_BASE_URL="https://api.mobizon.kz"
MOBIZON_FROM=""
MOBIZON_TIMEOUT_SECONDS="10"
MOBIZON_RETRIES="1"
MOBIZON_DRY_RUN="true"
```

Путь к конфигу можно переопределить переменной окружения `CONFIG_PATH` (по умолчанию `config/config.yaml`).
Секрет JWT можно задавать через `security.jwt_secret` в конфиге или через переменную окружения `JWT_SECRET`.
TTL access-токена настраивается через переменную окружения `ACCESS_TOKEN_TTL` (формат Go duration, например `2h`; по умолчанию `2h`).
Для удобства можно создать `.env` из `.env.example` и хранить там параметры, которые затем подставляются в `config.yaml` и/или используются при запуске.
Для signing flow используются два TTL: `sign_email_ttl_minutes` (email OTP/магическая ссылка) и `sign_session_ttl_minutes` (post-confirm sign session); если `sign_session_ttl_minutes` не задан, он наследуется из `sign_email_ttl_minutes`.

---

## Миграции БД

Применяй миграции последовательно из `db/migrations/*.up.sql` (в compose это делает сервис `migrate` через `scripts/run-migrations.sh`). Для ручного прогона:
```bash
./scripts/run-migrations.sh
```
> Важно: миграционный раннер подхватывает только файлы с суффиксом `.up.sql`/`.down.sql`.  
> Для audit-логов используется `db/migrations/022_audit_logs.up.sql`.

**Миграция содержит:**
- `roles`, `users` (+refresh-поля, телефон, флаг верификации)
- `leads`, `deals`, `documents`, `messages`, `tasks`
- `user_verifications` (для регистрации/логина по телефону): `code_hash`, `expires_at`, `attempts`, `confirmed`
- Индексы для производительности и уникальности
- Сиды для ролей (10/20/30/40/50)

> Готовый SQL у тебя уже есть в репозитории (последняя версия с `user_verifications` и `code_hash`).

---

## Аутентификация и безопасность

- **Пароли пользователей**: bcrypt.  
- **Коды подтверждения для регистрации**: **НЕ храним** в открытом виде — только `bcrypt`-хэш.  
- **TTL кода**: 5 минут (по умолчанию, настраивается в сервисе).  
- **Лимит попыток подтверждения**: max **5** (после этого код инвалидируется, нужен resend).  
- **Resend-троттлинг**: не более **3** раз за **10 минут** (на превышении — **429 Too Many Requests**).  
- **JWT**:  
  - Access: `ACCESS_TOKEN_TTL` (по умолчанию 2 часа), передаётся в `Authorization: Bearer <token>`  
  - Refresh: ~30 дней, **хранится в БД в hashed-виде**, **ротация** на `/auth/refresh`.  
- **Login блокируется**, если `is_verified=false` (телефон не подтверждён).
- В текущей реализации auth не использует server-side session/cookie/Redis: logout зависит от срока `exp` access JWT и срока refresh в БД.

---

## RBAC (роли и права)

| Роль (ID)   | Описание                                         |
|-------------|---------------------------------------------------|
| 10 `sales`  | Лиды/сделки свои, документы — отправка на ревью   |
| 20 `operations`    | Операционный доступ к бизнес-сущностям и документам |
| 30 `control`  | Широкий read-only доступ к бизнес-данным |
| 40 `leadership` | Полный доступ к бизнес-сущностям + подпись документов |
| 50 `system_admin` | Системное администрирование: роли/пользователи/интеграции/debug |

> Ограничения в middleware + в хендлерах (проверка владельца и/или повышенной роли).

Подробнее по канонической модели и policy-хелперам: `docs/rbac.md`.

---

## Эндпоинты

### Публичные
- `POST /register` — регистрация sales + код подтверждения  
- `POST /register/confirm` — подтвердить email (payload: `user_id`, `code`)  
- `POST /register/resend` — повторная отправка кода (payload: `user_id`)  
- `POST /auth/login` — логин (если `is_verified=false` → 403)  
- `POST /auth/refresh` — ротация refresh и выдача нового access  

### Защищённые (JWT)

**Users**
- `POST /users` (system_admin) — создать пользователя любой роли; опционально `is_verified=true` для мгновенной верификации (если поле не передано, поведение прежнее: `is_verified=false`)  
- `GET /users` (leadership/system_admin/control) — список  
- `GET /users/:id` (leadership/system_admin/control; обычный юзер — только себя)  
- `PUT /users/:id` — обновить (обычный юзер — только себя; поля верификации/роль — только system_admin) 
- `DELETE /users/:id` (system_admin)
- `GET /users/me` — enriched human profile: `first_name/last_name/middle_name/full_name`, `role`, `position`, `branch`, `telegram`, `legacy`
- create/update payload дополнен полями: `first_name`, `last_name`, `middle_name`, `position`, `branch_id`, `is_active`

### Branches (single-company model)

- `GET /branches` — список филиалов (все аутентифицированные роли)
- `GET /branches/:id` — детали филиала (`system_admin/leadership` любой; остальные — только свой)
- `POST /branches` — создать филиал (`system_admin`)
- `PUT /branches/:id` — обновить филиал (`system_admin`)
- `DELETE /branches/:id` — удалить филиал (`system_admin`)

### Branch-based data scope

- `branch_id` добавлен в ключевые сущности: `leads`, `deals`, `tasks`, `documents`, `chats`.
- Наследование:
  - `lead.branch_id` = филиал текущего пользователя
  - `deal.branch_id` наследуется из лида
  - `task.branch_id` наследуется из автора (fallback)
  - `document.branch_id` наследуется из сделки
  - `chat.branch_id` наследуется из создателя чата
- Elevated роли (`control`, `leadership`, `system_admin`) могут использовать list-фильтр `branch_id`.

**Roles** (system_admin)
- CRUD + счётчики

**Clients**
- `GET /clients` — общий список клиентов
- `GET /clients/individual?limit=&offset=&q=` — только физ. лица (`client_type=individual`)
- `GET /clients/company?limit=&offset=&q=` — только юр. лица (`client_type=legal`)
- Payload/response: базовые поля клиента + вложенные `individual_profile` / `legal_profile` (legacy flat payload остаётся совместимым).

**Leads / Deals**
- CRUD, конвертация лида в сделку, фильтры/пагинация, ограничения по владельцу для sales

**Documents**
- Создание по сделке, генерация/хранение файла, просмотр/скачивание с проверкой прав  
- `POST /documents/:id/submit` — отправка на ревью (sales/elevated)  
- `POST /documents/:id/review` — ревью (operations/leadership)  
- `POST /documents/:id/sign` — подпись (leadership)

**Tasks** (sales/operations/control/leadership/system_admin)
- CRUD

**Messages** (roles with chat access; см. `docs/rbac.md`)
- Отправка, список диалогов, история
- `GET /chats/users` — chat-scoped directory для выбора пользователя в личный чат (`q/query`, `limit`, `offset`, только safe-lite поля)
- `GET /chats` / `GET /chats/search` — теперь включают `counterparty` (для personal) и `participants_preview`/`member_profiles` (для group), чтобы UI не зависел от `/users`
- Для `control` (read-only) включено **узкое исключение** только для chat-actions: `POST /chats/personal`, `POST /chats/:id/messages`, `POST /chats/:id/read`; write-доступ к бизнес-сущностям остаётся закрытым.
- `GET /chats/users` возвращает `existing_personal_chat_id` и используется как основной chat picker для фронта (вместо privileged `/users`).
- UI rendering policy:
  - personal chat: использовать `counterparty`,
  - group chat: использовать `participants_preview` + `member_profiles`,
  - messages: использовать `sender_profile`.

**Подписание документов по коду** (доступ согласно документным policy checks; см. `docs/rbac.md`)

**Reports** (sales/operations/control/leadership/system_admin)
- `/reports/funnel`, `/reports/leads`, `/reports/revenue`, `/reports/revenue/export`
- `branch_id` query filter:
  - `control` / `leadership` / `system_admin` могут фильтровать отчёты по любому филиалу;
  - `sales` всегда получает отчёты только по `owner_id=self` и своему `branch_id`;
  - `operations` всегда получает отчёты своего `branch_id`.

---

## Коллекция Postman

Актуальные файлы:
- `postman/KUB API.postman_collection.json`
- `postman/KUB Local.postman_environment.json`
- `postman/README.md` (быстрый гайд по импорту и smoke flow)

Коллекция включает все текущие роуты (Auth, Users & Roles, Clients, Leads, Deals, Documents, Signing, Chats, Tasks, Reports, Integrations/Wazzup, Debug) и выстроена под локальный smoke flow на `http://localhost:4000`.

WebSocket (`GET /chats/:id/ws`) использует JWT из `Authorization: Bearer <token>` как основной способ. Query-параметр `token`/`access_token` поддерживается только как fallback для браузерных клиентов и не должен логироваться или проксироваться в access-логи.

Базовый сценарий:
1. Импортируй collection + environment.
2. Запусти `Auth / Login` (access/refresh token сохраняются автоматически).
3. Выполняй smoke по папкам сверху вниз.

---

## Требования/запуск в проде

- `GIN_MODE=release`
- Задать безопасный `security.jwt_secret` или переменную `JWT_SECRET`
- Настроить **пароль приложения** для SMTP (Mail.ru/Gmail и т. п.)
- Включить **логирование в файл** и безопасные заголовки (CORS/CSRF по контексту)
- Регулярные **бэкапы БД**
- Ротация ключей/секретов по регламенту

## Деплой через Docker Compose

1. Скопировать пример конфига:
```bash
cp config/config.example.yaml config/config.yaml
```
2. Поднять стек единым compose-файлом:
```bash
docker compose -f docker-compose.prod.yml up -d --build
```
3. Полезные команды:
```bash
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs -f
docker compose -f docker-compose.prod.yml down --remove-orphans
```
4. По умолчанию стартуют `postgres`, `migrate`, `api`; опционально можно включать профили `redis` и `nginx`.

## Деплой через systemd (без Docker)

1. Собрать бинарник:
```bash
make build
```
   или скачать артефакт CI/CD. Скопировать `bin/turcompany` в `/usr/local/bin/turcompany`.
2. Скопировать `assets/` и `config/` в `/opt/turcompany`, создать каталоги `files/`, `files/pdf`, `files/docx`, `files/excel`:
```bash
sudo mkdir -p /opt/turcompany/files /opt/turcompany/files/pdf /opt/turcompany/files/docx /opt/turcompany/files/excel
```
3. Скопировать `config/config.example.yaml` в `/opt/turcompany/config/config.yaml` и заполнить значения.
4. Создать файл `/etc/turcompany.env` с переменными окружения (например, `GIN_MODE=release` или `CONFIG_PATH=/opt/turcompany/config/config.yaml`).
5. Положить юнит `docs/deploy/systemd/turcompany.service` в `/etc/systemd/system/turcompany.service`, при необходимости поправить `ExecStart`/`WorkingDirectory`.
6. Применить и запустить сервис:
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now turcompany.service
```

## Проверка генерации документов

1. Создать тестовых клиента/сделку (через API или админку).
2. Отправить запрос на генерацию, например `POST /documents/create-from-client` (DocumentHandler.CreateDocumentFromClient) через Postman с нужным типом документа и заполненными полями.
3. Убедиться, что в каталоге `files/pdf`, `files/docx` или `files/excel` появились новые файлы.
4. Если `libreoffice.enable=true` — убедиться, что `soffice` доступен по пути из конфига; при ошибке в логах будет строка вида `libreoffice conversion failed: ...`.

### JSON payload для POST /documents/create-from-client

```json
{
  "client_id": 123,
  "client_type": "individual",
  "deal_id": 456,
  "doc_type": "cancel_appointment",
  "extra": {
    "reason_code": "R1",
    "CANCEL_REASON_TEXT": "Личные обстоятельства",
    "CANCEL_OTHER_TEXT": ""
  }
}
```

- Верхний уровень:
  - `client_id` — **обязательное**, `int` (`binding:"required"`).
  - `client_type` — **обязательное**, `individual|legal` (`binding:"required"`).
  - `deal_id` — опциональное, `int` (если `0`, берётся последняя сделка по точной typed-ссылке `client_id+client_type`).
  - `doc_type` — **обязательное**, `string` (`binding:"required"`).
  - `extra` — опциональное `object<string,string>`.
- Обязательные поля `extra` по `doc_type`:
  - `cancel_appointment` → `reason_code`.
  - `refund_application` → `reason_code`.
  - `pause_application` → `reason_code`.
  - для остальных `doc_type` обязательных полей внутри `extra` нет (но есть опциональные ключи в `internal/services/document_registry.go`).
- Для всех `doc_type` сервис дополнительно проверяет обязательные данные клиента/сделки: `full_name`, `iin_or_bin`, `address`, `phone`, `contract_number`.
- Сервис валидирует typed client reference и возвращает ошибку при mismatch (`client_type does not match stored client type`), а также при попытке скрестить `deal_id` от другого клиента.

### Typed client contract (deals/leads/documents)

- `POST /deals` и `PUT /deals/:id` требуют `client_id` + `client_type`.
- `PUT /leads/:id/convert` требует `client_id` + `client_type`.
- `POST /documents/create-from-client` требует `client_id` + `client_type`.

### Immutability

`client_type` существующего клиента считается неизменяемым: `PUT/PATCH /clients/:id` не может менять `individual <-> legal`.

### DEBUG для проблемного payload

Для диагностики ошибок биндинга включите переменную окружения:

```bash
DOCS_DEBUG_SCHEMA=1
```

Тогда при `Invalid payload` endpoint залогирует:
- полный текст ошибки биндинга (`err.Error()`),
- сырое тело запроса (обрезается до 8192 байт).

## Логи и конфигурация

- Приложение пишет в stdout/stderr (journalctl/Docker читают их напрямую).
- При старте выводится конфигурация: `server.port`, `files.root_dir`, пути к шаблонам, флаги `telegram.enable` и `libreoffice.enable`. Секреты/пароли в лог не выводятся.

---

## Траблшутинг

- **SMTP 535**: Mail.ru ругается — нужен **пароль приложения** (включить 2FA и создать app-password).  
- **/register/resend → 429**: сработал троттлинг (частые запросы). Подожди минуту или снизь частоту.  
- **/register/confirm → 400**: неверный код, код просрочен или нет активной верификации. Сделай resend.

---

## Структура репо

```
.
├─ cmd/web/main.go               # точка входа
├─ internal/
│  ├─ app/                       # запуск приложения
│  ├─ authz/                     # роли/права
│  ├─ config/                    # config loader (yaml)
│  ├─ handlers/                  # HTTP-хендлеры
│  ├─ middleware/                # JWT, RBAC, read-only guard
│  ├─ models/                    # модели
│  ├─ pdf/                       # генерация PDF
│  ├─ repositories/              # SQL-репозитории (Postgres)
│  ├─ routes/                    # роутинг Gin
│  ├─ services/                  # бизнес-логика (Auth, User и т.д.)
│  └─ utils/                     # утилиты (refresh token, phone utils)
├─ assets/fonts/DejaVuSans.ttf   # шрифт для PDF
├─ files/                        # хранилище документов (локально)
├─ config/config.example.yaml    # пример конфигурации (копируется в config.yaml)
└─ db/migrations/001_base_schema.sql
```

---

## Примеры запросов (быстрый сценарий)

1) **Регистрация (sales)**
```bash
curl -X POST http://localhost:4000/register   -H "Content-Type: application/json"   -d '{"first_name":"Sales","last_name":"One","branch_id":1,"company_name":"Acme","bin_iin":"123","email":"sales1@example.com","password":"sales12345","phone":"+77000000000"}'
```

2) **Подтверждение**
```bash
curl -X POST http://localhost:4000/register/confirm   -H "Content-Type: application/json"   -d '{"user_id":1,"code":"123456"}'
```

3) **Повторная отправка кода**
```bash
curl -X POST http://localhost:4000/register/resend   -H "Content-Type: application/json"   -d '{"user_id":1}'
```

4) **Вход**
```bash
curl -X POST http://localhost:4000/auth/login   -H "Content-Type: application/json"   -d '{"email":"sales1@example.com","password":"sales12345"}'
```

5) **Ротация refresh**
```bash
curl -X POST http://localhost:4000/auth/refresh   -H "Content-Type: application/json"   -d '{"refresh_token":"<your_refresh_token>"}'
```
### Подписание документа (Email + Telegram)
**Создать сессию подписи (требует JWT):**
```bash
curl -X POST http://localhost:4000/api/v1/sign/sessions \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"document_id": 123, "phone": "+77001234567"}'
```

Ответ: `{ "status": "sent" }` (в dry_run дополнительно вернётся token/sign_url).

**Подтвердить код (публичный):**
```bash
curl -X POST http://localhost:4000/api/v1/sign/sessions/token/{token}/verify \
  -H "Content-Type: application/json" \
  -d '{"code": "123456"}'
```

**Подписать документ (публичный):**
```bash
curl -X POST http://localhost:4000/api/v1/sign/sessions/token/{token}/sign \
  -H "Content-Type: application/json" \
  -d '{"agree": true}'
```

**Публичная HTML-страница:**
```
GET /api/v1/sign/sessions/id/{id}/page?token={token}
```

---

## Что проверить после деплоя

- ✅ SIGN_BASE_URL указывает на публичный домен с доступным `/api/v1/sign/sessions/id/{id}/page?token={token}`.
- ✅ Все `*.up.sql` миграции из `db/migrations` применены (включая split clients profiles и Wazzup data layer).
- ✅ Проверить rate limit: >3 сессий за 10 минут на документ/телефон возвращает 429.
- ✅ Проверить TTL по фактическому конфигу: после `sign_email_ttl_minutes` истекает verify, после `sign_session_ttl_minutes` истекает sign-сессия (410).
