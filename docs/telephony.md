# Telephony — Binotel Integration

## 1. Требуемые env-переменные

| Переменная              | Обязательно | Описание                                            |
|-------------------------|-------------|-----------------------------------------------------|
| `BINOTEL_API_KEY`       | **Да**      | API-ключ Binotel (личный кабинет → REST API). Без него история звонков не загружается и кнопка «Позвонить» не работает. |
| `BINOTEL_API_SECRET`    | **Да**      | API-секрет Binotel                                  |
| `BINOTEL_WEBHOOK_SECRET`| Рекомендуется | Секрет для проверки входящих webhook-запросов (передаётся как `?token=` в URL webhook-а). Если не задан — webhook принимает любые запросы. |
| `BINOTEL_ENABLED`       | Нет         | `true`/`false` (флаг для будущей логики включения/отключения) |

> **Важно:** история звонков наполняется двумя путями: (1) фоновая синхронизация каждые 5 минут через REST API (`stats/all-incoming-calls-since` + `all-outgoing-calls-since`), которая требует `BINOTEL_API_KEY`/`BINOTEL_API_SECRET`; (2) webhook в реальном времени. Если раньше «ничего не показывалось» — почти всегда потому, что REST-ключи не были заданы, и таблица звонков оставалась пустой. Кнопка «Обновить из Binotel» на странице «Телефония» запускает синхронизацию вручную (доступно admin/management), либо `POST /api/v1/telephony/sync`.
>
> REST API v4.0: base `https://api.binotel.com/api/4.0/{method}.json`, авторизация — поля `key`+`secret` в теле JSON, успех — `{"status":"success"}`.

Или через `config/config.yaml`:

```yaml
binotel:
  webhook_secret: "your-secret-here"
  api_key: ""
  api_secret: ""
  enabled: true
```

## 2. Webhook URL для настройки в Binotel

Укажите в настройках Binotel:

```
POST https://api.kubcrm.kz/api/v1/integrations/binotel/webhook
```

### Защита webhook

Binotel должен передавать секрет одним из способов:

**Вариант A — заголовок (рекомендуется):**
```
X-Binotel-Secret: your-secret-here
```

**Вариант B — query-параметр:**
```
POST /api/v1/integrations/binotel/webhook?token=your-secret-here
```

Если `BINOTEL_WEBHOOK_SECRET` не задан, запросы принимаются без проверки
(только для локальной разработки — в prod обязательно задать секрет).

## 3. Пример curl для тестирования

```bash
curl -X POST https://api.kubcrm.kz/api/v1/integrations/binotel/webhook \
  -H "Content-Type: application/json" \
  -H "X-Binotel-Secret: your-secret-here" \
  -d '{
    "generalCallID": "test-001",
    "eventType": "call_start",
    "externalNumber": "+77001234567",
    "internalNumber": "101",
    "callType": 0,
    "startTime": 1700000000
  }'
```

Ожидаемый ответ:
```json
{"status":"ok","call_id":1,"is_new":true}
```

## 4. Как проверить, что звонок создался

```bash
# GET список всех звонков (требует JWT)
curl -H "Authorization: Bearer <token>" \
  https://api.kubcrm.kz/api/v1/telephony/calls

# GET один звонок по ID
curl -H "Authorization: Bearer <token>" \
  https://api.kubcrm.kz/api/v1/telephony/calls/1

# GET звонки конкретного клиента
curl -H "Authorization: Bearer <token>" \
  https://api.kubcrm.kz/api/v1/clients/42/calls

# GET звонки конкретного лида
curl -H "Authorization: Bearer <token>" \
  https://api.kubcrm.kz/api/v1/leads/77/calls
```

## 5. Логика автосоздания лида

При входящем звонке (`callType=0` или `eventType=incoming_call`):

1. Нормализуем номер — убираем все нецифровые символы (`+7(700) 123-45-67` → `77001234567`).
2. Ищем клиента по `clients.primary_phone` или `clients.phone` (regexp strip non-digits).
   - Нашли → привязываем `client_id`, лид не создаём.
3. Ищем лид по `leads.phone` (нормализованный).
   - Нашли → привязываем `lead_id`.
4. Если ни клиент, ни лид не найдены → создаём новый лид:
   - `source = 'binotel'`
   - `title = 'Входящий звонок +77001234567'`
   - `status = 'new'`
   - `owner_id` = менеджер из `internalNumber` (если совпал пользователь), иначе `NULL`
   - `branch_id` = берётся из найденного менеджера или `NULL`
5. Повторный webhook с тем же `generalCallID` → upsert (обновление записи, лид/клиент не дублируются).

Для **исходящих звонков** (`callType=1`) лид автоматически **не создаётся**.

## 6. Idempotency

Webhook idempotent: повторная доставка с тем же `generalCallID` (или `callID`) обновляет запись,
но не создаёт дубль. Признак дубля — `is_new: false` в ответе.

Если `generalCallID` пустой — новая запись создаётся при каждом запросе (без гарантии уникальности).

## 7. Типы событий Binotel (поддерживаются)

| eventType         | Статус записи   |
|-------------------|-----------------|
| `call_start`      | `incoming`      |
| `call_answer`     | `answered`      |
| `call_end`        | `completed`     |
| `missed_call`     | `missed`        |
| Неизвестный       | `unknown`       |

`disposition` поля: `answered` → `answered`, `noanswer`/`busy` → `missed`, `failed` → `failed`.

## 8. Структура таблицы telephony_calls

| Поле              | Тип          | Описание                              |
|-------------------|--------------|---------------------------------------|
| `id`              | BIGSERIAL    | Первичный ключ                        |
| `provider`        | TEXT         | Всегда `binotel`                     |
| `external_call_id`| TEXT NULL    | `generalCallID` из Binotel            |
| `direction`       | TEXT         | `inbound` / `outbound`               |
| `status`          | TEXT         | incoming/missed/answered/completed/…  |
| `phone`           | TEXT         | Исходный номер из payload             |
| `normalized_phone`| TEXT NULL    | Нормализованный (только цифры)        |
| `client_id`       | BIGINT NULL  | FK → clients.id                       |
| `lead_id`         | BIGINT NULL  | FK → leads.id                        |
| `manager_id`      | INT NULL     | FK → users.id                        |
| `branch_id`       | INT NULL     | FK → branches.id                     |
| `started_at`      | TIMESTAMPTZ  | Время начала (из payload Unix)        |
| `answered_at`     | TIMESTAMPTZ  | Время ответа                         |
| `ended_at`        | TIMESTAMPTZ  | Время завершения                      |
| `duration_seconds`| INT NULL     | Длительность в секундах               |
| `recording_url`   | TEXT NULL    | URL записи разговора                  |
| `raw_payload`     | JSONB        | Полный payload от Binotel             |

## 9. Разрешения

Все приватные endpoints требуют JWT + `telephony.view`:

| Роль              | Видит звонки                              |
|-------------------|-------------------------------------------|
| admin             | Все                                       |
| management        | Все (TODO: ограничить по branch)          |
| sales             | Ограничено своим branch_id (TODO)         |
| visa / partner    | Ограничено своим branch_id (TODO)         |
| hr / legal        | Ограничено своим branch_id (TODO)         |
| quality_control   | Ограничено своим branch_id (TODO)         |

> TODO: Полноценный scoping по branch/department реализовать в следующей итерации.

## 10. Ограничения MVP

- **Исходящие звонки из CRM не реализованы** (нет кнопки "Позвонить" в интерфейсе).
- **Запись не скачивается в S3** — `recording_url` хранится как ссылка на сервер Binotel.
- **Feed-события** при создании звонка не генерируются.
- **Нет полноценного скоупинга по роли** — admin/management видят всё, остальные пока тоже.
- Поиск менеджера по extension работает только если телефон пользователя заполнен в CRM.
- Вкладка "Звонки" в карточке клиента: ссылка `/telephony?client_id=ID` не реализована как отдельная вкладка — нужно доработать.
