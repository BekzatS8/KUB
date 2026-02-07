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

- ✅ Регистрация пользователя с верификацией телефона
- ✅ Троттлинг и защита от брутфорса для кода подтверждения
- ✅ Безопасное хранение кода (bcrypt-хэш), TTL, попытки, resend-лимиты
- ✅ JWT-аутентификация + ротация refresh-токена
- ✅ RBAC: sales / staff / operations / audit / management / admin
- ✅ Лиды, сделки, задачи, сообщения; документы с подтверждением подписи кодом
- ✅ Отчёты и фильтры
- ✅ Скачивание/просмотр PDF из защищённого стора

---

## Архитектура и стек

- **Go 1.22+**, **Gin** (HTTP, middleware)
- **PostgreSQL** (модель данных и индексы заточены под RBAC/аудит/фильтры)
- **SMTP** (почта)
- **JWT**: access (короткий), refresh (длинный, хранится в БД с ротацией)
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
```

Путь к конфигу можно переопределить переменной окружения `CONFIG_PATH` (по умолчанию `config/config.yaml`).
Секрет JWT можно задавать через `security.jwt_secret` в конфиге или через переменную окружения `JWT_SECRET`.
Для удобства можно создать `.env` из `.env.example` и хранить там параметры, которые затем подставляются в `config.yaml` и/или используются при запуске.

---

## Миграции БД

Применяй миграции вручную (автоприменения нет). В staging/prod последовательно выполни:
```bash
psql "$DATABASE_URL" -f db/migrations/001_init.up.sql
psql "$DATABASE_URL" -f db/migrations/002_sign_sessions.up.sql
psql "$DATABASE_URL" -f db/migrations/999_audit_logs.sql
```
**Миграция содержит:**
- `roles`, `users` (+refresh-поля, телефон, флаг верификации)
- `leads`, `deals`, `documents`, `messages`, `tasks`
- `user_verifications` (для регистрации/логина по телефону): `code_hash`, `expires_at`, `attempts`, `confirmed`
- Индексы для производительности и уникальности
- Сиды для ролей (10/15/20/30/40/50)

> Готовый SQL у тебя уже есть в репозитории (последняя версия с `user_verifications` и `code_hash`).

---

## Аутентификация и безопасность

- **Пароли пользователей**: bcrypt.  
- **Коды подтверждения для регистрации**: **НЕ храним** в открытом виде — только `bcrypt`-хэш.  
- **TTL кода**: 5 минут (по умолчанию, настраивается в сервисе).  
- **Лимит попыток подтверждения**: max **5** (после этого код инвалидируется, нужен resend).  
- **Resend-троттлинг**: не более **3** раз за **10 минут** (на превышении — **429 Too Many Requests**).  
- **JWT**:  
  - Access: ~15 минут  
  - Refresh: ~30 дней, **хранится в БД** у пользователя, **ротация** на `/refresh`.  
- **Login блокируется**, если `is_verified=false` (телефон не подтверждён).

---

## RBAC (роли и права)

| Роль (ID)   | Описание                                         |
|-------------|---------------------------------------------------|
| 10 `sales`  | Лиды/сделки свои, документы — отправка на ревью   |
| 15 `staff`  | Доступ к задачам и сообщениям                     |
| 20 `ops`    | Проверка/ревью документов                         |
| 30 `audit`  | Read-only (частичная маскировка чувствит. полей)  |
| 40 `mgmt`   | Полный доступ + подпись документов                |
| 50 `admin`  | Полный доступ + управление ролями/пользователями  |

> Ограничения в middleware + в хендлерах (проверка владельца и/или повышенной роли).

---

## Эндпоинты

### Публичные
- `POST /register` — регистрация sales + код подтверждения  
- `POST /register/confirm` — подтвердить email (payload: `user_id`, `code`)  
- `POST /register/resend` — повторная отправка кода (payload: `user_id`)  
- `POST /login` — логин (если `is_verified=false` → 403)  
- `POST /refresh` — ротация refresh и выдача нового access  

### Защищённые (JWT)

**Users**
- `POST /users` (admin) — создать пользователя любой роли  
- `GET /users` (mgmt/admin/audit) — список  
- `GET /users/:id` (mgmt/admin/audit; обычный юзер — только себя)  
- `PUT /users/:id` — обновить (обычный юзер — только себя; поля верификации/роль — только админ)  
- `DELETE /users/:id` (admin)

**Roles** (admin)
- CRUD + счётчики

**Leads / Deals**
- CRUD, конвертация лида в сделку, фильтры/пагинация, ограничения по владельцу для sales

**Documents**
- Создание по сделке, генерация/хранение файла, просмотр/скачивание с проверкой прав  
- `POST /documents/:id/submit` — отправка на ревью (sales/elevated)  
- `POST /documents/:id/review` — ревью (ops/mgmt/admin)  
- `POST /documents/:id/sign` — подпись (mgmt/admin)

**Tasks** (staff/ops/mgmt/admin)
- CRUD

**Messages** (staff/ops/mgmt/admin)
- Отправка, список диалогов, история

**Подписание документов по коду** (sales/ops/mgmt/admin)

**Reports** (audit/ops/mgmt/admin)
- `/reports/summary`, `/reports/leads/filter`, `/reports/deals/filter`

---

## Коллекция Postman

Коллекция включает **все текущие роуты**, три шага **register → confirm → login**, ротацию refresh и примеры запросов по сущностям.

Переменные коллекции:
- `baseUrl` — `http://localhost:4000`
- `accessToken`, `refreshToken` — выставляются автоматически после `login`/`refresh`

Импортируй актуальный JSON (обновлён под твоё приложение).

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
cp config/config.yaml config/config.yaml
```
2. Собрать и поднять сервисы:
```bash
make docker-build
docker-compose up -d
```
   По умолчанию используется `config/config.yaml`, каталоги `assets/` и `files/` пробрасываются в контейнер. Корень хранилища — `/opt/turcompany/files` (смонтирован как `./files`).
3. Проверить, что Postgres поднялся (порт `5432`), а приложение слушает порт `4000`.
4. При необходимости включить LibreOffice внутри контейнера (добавить пакет) или установить на хосте и пробросить бинарь в образ.

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
2. Отправить запрос на генерацию, например `POST /documents/from-client` (DocumentHandler.CreateDocumentFromClient) через Postman с нужным типом документа и заполненными полями.
3. Убедиться, что в каталоге `files/pdf`, `files/docx` или `files/excel` появились новые файлы.
4. Если `libreoffice.enable=true` — убедиться, что `soffice` доступен по пути из конфига; при ошибке в логах будет строка вида `libreoffice conversion failed: ...`.

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
curl -X POST http://localhost:4000/register   -H "Content-Type: application/json"   -d '{"company_name":"Acme","bin_iin":"123","email":"sales1@example.com","password":"sales12345","phone":"+77000000000"}'
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
curl -X POST http://localhost:4000/login   -H "Content-Type: application/json"   -d '{"email":"sales1@example.com","password":"sales12345"}'
```

5) **Ротация refresh**
```bash
curl -X POST http://localhost:4000/refresh   -H "Content-Type: application/json"   -d '{"refresh_token":"<your_refresh_token>"}'
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
curl -X POST http://localhost:4000/api/v1/sign/sessions/{token}/verify \
  -H "Content-Type: application/json" \
  -d '{"code": "123456"}'
```

**Подписать документ (публичный):**
```bash
curl -X POST http://localhost:4000/api/v1/sign/sessions/{token}/sign \
  -H "Content-Type: application/json" \
  -d '{"agree": true}'
```

**Публичная HTML-страница:**
```
GET /sign/{token}
```

---

## Что проверить после деплоя

- ✅ SIGN_BASE_URL указывает на публичный домен с доступным `/sign/{token}`.
- ✅ Миграция `002_sign_sessions.up.sql` применена.
- ✅ Проверить rate limit: >3 сессий за 10 минут на документ/телефон возвращает 429.
- ✅ Проверить TTL: по истечении 10 минут verify/sign возвращает 410.
