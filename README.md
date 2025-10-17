# KUB API — CRM-бэкенд с RBAC, SMS-верификацией и документооборотом

KUB — это REST API на **Go + Gin**, с ролевой моделью доступа (**RBAC**), безопасной регистрацией через **SMS-код**, управлением лидами/сделками/задачами/документами, и отчётами.  
Хранилище — **PostgreSQL**. SMS-шлюз — **Mobizon**. PDF-генерация — встроенная.

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
- [Траблшутинг](#траблшутинг)  
- [Структура репо](#структура-репо)  
- [Примеры запросов (быстрый сценарий)](#примеры-запросов-быстрый-сценарий)

---

## Возможности

- ✅ Регистрация пользователя с SMS-верификацией телефона
- ✅ Троттлинг и защита от брутфорса для кода подтверждения
- ✅ Безопасное хранение кода (bcrypt-хэш), TTL, попытки, resend-лимиты
- ✅ JWT-аутентификация + ротация refresh-токена
- ✅ RBAC: sales / staff / operations / audit / management / admin
- ✅ Лиды, сделки, задачи, сообщения; документы с SMS-подписанием
- ✅ Отчёты и фильтры
- ✅ Скачивание/просмотр PDF из защищённого стора

---

## Архитектура и стек

- **Go 1.22+**, **Gin** (HTTP, middleware)
- **PostgreSQL** (модель данных и индексы заточены под RBAC/аудит/фильтры)
- **Mobizon** (SMS), **SMTP** (почта)
- **JWT**: access (короткий), refresh (длинный, хранится в БД с ротацией)
- **bcrypt** для паролей и верификационных кодов
- Генерация PDF (пользовательские шаблоны/шрифты)

---

## Быстрый старт

1) Установи Go и PostgreSQL.  
2) Создай БД и накатай миграции (см. ниже).  
3) Заполни `config/config.yaml` (см. пример).  
4) Запуск:
```bash
go run cmd/web/main.go
# или
GIN_MODE=release go run cmd/web/main.go
```
Сервер поднимется на `http://localhost:<port>` (по умолчанию 4000).

---

## Конфиг

Файл: `config/config.yaml`
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

mobizon:
  api_key: "MOBIZON_API_KEY"
  sender_id: ""                     # опционально
  dry_run: false                    # true = не отправлять фактически SMS
```

> **Важно:** JWT-секрет сейчас жёстко прописан в `internal/middleware/jwt.go` (переменная `JWTKey`). Для прода — вынести в ENV/конфиг.

---

## Миграции БД

Положи миграцию в `db/migrations/001_base_schema.sql` и выполни:
```bash
psql "$DATABASE_URL" -f db/migrations/001_base_schema.sql
```
**Миграция содержит:**
- `roles`, `users` (+refresh-поля, телефон, флаг верификации)
- `leads`, `deals`, `documents`, `messages`, `tasks`
- `sms_confirmations` (для документов)
- `user_verifications` (для регистрации/логина по телефону): `code_hash`, `expires_at`, `attempts`, `confirmed`
- Индексы для производительности и уникальности
- Сиды для ролей (10/15/20/30/40/50)

> Готовый SQL у тебя уже есть в репозитории (последняя версия с `user_verifications` и `code_hash`).

---

## Аутентификация и безопасность

- **Пароли пользователей**: bcrypt.  
- **SMS-коды для регистрации**: **НЕ храним** в открытом виде — только `bcrypt`-хэш.  
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
- `POST /register` — регистрация sales + SMS код  
- `POST /register/confirm` — подтвердить телефон (код)  
- `POST /register/resend` — повторная отправка кода (ограничения по троттлингу)  
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

**SMS (для документов)** (sales/ops/mgmt/admin)
- `POST /sms/send`, `POST /sms/resend`, `POST /sms/confirm`, `GET /sms/latest/:document_id`, `DELETE /sms/:document_id`

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
- Вынести `JWTKey` в переменную окружения/конфиг  
- Настроить **пароль приложения** для SMTP (Mail.ru/Gmail и т. п.)  
- Включить **логирование в файл** и безопасные заголовки (CORS/CSRF по контексту)  
- Регулярные **бэкапы БД**  
- Ротация ключей/секретов по регламенту  

---

## Траблшутинг

- **SMTP 535**: Mail.ru ругается — нужен **пароль приложения** (включить 2FA и создать app-password).  
- **/register/resend → 429**: сработал троттлинг (больше 3 запросов за 10 минут). Подожди 10 минут или снизь частоту.  
- **/register/confirm → 400**: неверный/просроченный код или превышен лимит попыток (5). Сделай resend.

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
│  ├─ services/                  # бизнес-логика (Auth, User, SMS и т.д.)
│  └─ utils/                     # утилиты (refresh token, SMS клиент)
├─ assets/fonts/DejaVuSans.ttf   # шрифт для PDF
├─ files/                        # хранилище документов (локально)
├─ config/config.yaml            # конфигурация
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

3) **Вход**
```bash
curl -X POST http://localhost:4000/login   -H "Content-Type: application/json"   -d '{"email":"sales1@example.com","password":"sales12345"}'
```

4) **Ротация refresh**
```bash
curl -X POST http://localhost:4000/refresh   -H "Content-Type: application/json"   -d '{"refresh_token":"<your_refresh_token>"}'
```
