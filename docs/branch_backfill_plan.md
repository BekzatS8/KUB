# Проблема Б: план backfill branch_id

> Статус: **ПЛАН ТОЛЬКО** — не реализован, не выполнен.  
> Перед любым действием запустить `diagnostic_branch_null.sql` и получить цифры.  
> Выбор варианта — за заказчиком после анализа диагностики.

---

## 1. Пользователи (users.branch_id IS NULL)

### Почему возникает

Два источника:

1. **`Register()`** (`user_handler.go:869`) — публичный эндпоинт регистрации всегда создаёт `BranchID: nil`, `RoleID: sales(10)`. Ни одной проверки филиала нет.
2. **Данные до migration 026** — `branch_id` добавлен позже; существующие пользователи остались с NULL, и в migration 026 явно написано: *«backfill из legacy-полей невозможен»*.

### Варианты

#### Вариант А — Назначить дефолтный филиал (quick fix)
Найти ID наименьшего активного филиала (`SELECT id FROM branches WHERE is_active ORDER BY id LIMIT 1`),  
обновить всех NULL-пользователей одним `UPDATE`.

**Плюсы:** мгновенно устраняет ErrForbidden, pipeline становится видим.  
**Минусы:** если branch-scoped роль (sales/visa/quality_control) реально работает в другом филиале — они увидят не свои данные, пока admin не поправит вручную. Риск утечки данных между филиалами.  
**Когда применимо:** все NULL-пользователи = admin(50)/management(40) ИЛИ компания однофилиальная.

#### Вариант Б — Заблокировать вход до ручного назначения (safe block)
В `auth_handler.go` (Login) после успешной проверки пароля добавить проверку:  
если `roleID ∈ {10, 30, 60}` (branch-scoped роли) И `branch_id IS NULL` — вернуть  
`403 { "code": "branch_required", "message": "Филиал не назначен. Обратитесь к администратору." }`.

Фронтенд отображает специальный экран вместо dashboard.

**Плюсы:** нет риска видимости чужих данных; явное сообщение об ошибке.  
**Минусы:** пользователи не смогут войти, пока admin не назначит филиал вручную через `/users/:id` PUT.  
**Когда применимо:** таких пользователей мало (по §9 диагностики), admin может пройтись по списку за час.

#### Вариант В — Гибрид: дефолт для admin/management, блокировка для scoped
Комбинация А и Б:
- `role_id IN (40, 50)` → назначить дефолтный филиал автоматически (не ломают scope).
- `role_id IN (10, 30, 60, 70, 80, 90)` с NULL → блокировать вход до ручного назначения.

**Плюсы:** минимальный ущерб + нет утечки данных.  
**Когда применимо:** смешанный состав NULL-пользователей — часть admin/management, часть scoped.

### Точка application-level валидации (где ставить, без реализации сейчас)

| Место | Файл:строка | Что добавить |
|-------|-------------|-------------|
| Создание пользователя (admin) | [user_handler.go:294](../internal/handlers/user_handler.go#L294) | Уже есть `h.validateBranchForRole` — покрывает non-admin roles ✓ |
| **Само-регистрация** | [user_handler.go:869](../internal/handlers/user_handler.go#L869) | `BranchID: nil` без проверки — **основная дыра**. Либо закрыть `/register` (публичный эндпоинт не нужен если только admin создаёт юзеров), либо запретить роль sales без branch_id |
| Логин (блокировка) | `auth_handler.go` (после password check) | Добавить проверку branch_id для scoped-ролей (Вариант Б) |

---

## 2. Сущности (leads / deals / clients / tasks / documents / chats)

Все таблицы уже backfill-ились при добавлении колонки (migrations 027, 030, 031).  
NULL-записи остались только там, где `owner.branch_id` тоже был NULL в момент миграции.

### Стратегия backfill сущностей

**Шаг 1 — повторить owner-backfill** (идемпотентно, те же запросы что в migration 027/030/031):

```sql
-- Лиды: из owner
UPDATE leads l SET branch_id = u.branch_id
FROM users u WHERE l.owner_id = u.id AND l.branch_id IS NULL AND u.branch_id IS NOT NULL;

-- Сделки: из лида, затем из owner
UPDATE deals d SET branch_id = l.branch_id
FROM leads l WHERE d.lead_id = l.id AND d.branch_id IS NULL AND l.branch_id IS NOT NULL;

UPDATE deals d SET branch_id = u.branch_id
FROM users u WHERE d.owner_id = u.id AND d.branch_id IS NULL AND u.branch_id IS NOT NULL;

-- Клиенты: из owner
UPDATE clients c SET branch_id = u.branch_id
FROM users u WHERE c.owner_id = u.id AND c.branch_id IS NULL AND u.branch_id IS NOT NULL;

-- Задачи: из creator
UPDATE tasks t SET branch_id = u.branch_id
FROM users u WHERE t.creator_id = u.id AND t.branch_id IS NULL AND u.branch_id IS NOT NULL;

-- Документы: из deal
UPDATE documents dc SET branch_id = d.branch_id
FROM deals d WHERE dc.deal_id = d.id AND dc.branch_id IS NULL AND d.branch_id IS NOT NULL;

-- Чаты: из creator
UPDATE chats c SET branch_id = u.branch_id
FROM users u WHERE c.creator_id = u.id AND c.branch_id IS NULL AND u.branch_id IS NOT NULL;
```

**Шаг 2 — fallback для неисправимых** (если после шага 1 ещё остались NULL):  
Назначить DEFAULT_BRANCH_ID (из §8 диагностики). Эти записи "потеряны" — их владелец тоже был без филиала.  
Решение принимает заказчик: либо дефолтный филиал, либо оставить NULL (тогда они невидимы для scoped-ролей навсегда).

**Шаг 3 — telephony_calls**: `branch_id` заполняется из `manager_id.branch_id` по аналогии.  
Звонки без manager_id — оставить NULL или назначить дефолт.

### Application-level: где ставить NOT NULL при создании сущности

| Сущность | Файл | Точка |
|----------|------|-------|
| Lead | `lead_handler.go` → create | Установить `lead.BranchID = &user.BranchID` из токена до вставки |
| Deal | `deal_handler.go` → create | Унаследовать от `lead.BranchID` |
| Client | `client_handler.go` → create | Установить `client.BranchID = &user.BranchID` из токена |
| Task | `task_handler.go` → create | Установить `task.BranchID = &user.BranchID` из токена |
| Chat | `chat_handler.go` → create | Установить `chat.BranchID = &user.BranchID` из токена |
| Document | создаётся через deal | Унаследовать от `deal.BranchID` |

> Это указание точек, **не реализация**. Само присвоение делается в рамках Проблемы Б.

---

## 3. Последовательность действий (рекомендуемая)

```
1. Запустить diagnostic_branch_null.sql на проде → получить цифры (§1–§11).
2. По §9: решить что делать с каждым NULL-пользователем (Вариант А/Б/В).
3. Выполнить users backfill (вручную или скриптом).
4. Выполнить сущности backfill (шаг 1 выше).
5. Проверить §2 диагностики повторно — убедиться что NULL% упал.
6. Закрыть /register или добавить валидацию — остановить будущий приток NULL.
7. Добавить NOT NULL в миграции (Проблема Б финальная).
```

---

## 4. Что НЕ тронуто в этом документе

- `scope.go`, `branch_id` nullability в схеме — **не изменены**.
- Миграции — **не написаны**.
- Код валидации — **не изменён**.
- `TestCanonicalRoleIDsLocked` — **зелёный, не затронут**.
