# RBAC (каноническая модель)

## Source of truth

Единый источник ролей и policy-хелперов: `internal/authz/roles.go`.
Матрица прав: `internal/authz/permissions.go`.
Область видимости данных: `internal/services/scope.go`.

## Роли и role_id

| role_id | Канонический code  | Назначение |
|--------:|--------------------|------------|
| 10      | `sales`            | Отдел продаж: лиды/сделки/клиенты/документы в своём отделе |
| 30      | `quality_control`  | Контроль качества: read-only по всем бизнес-данным; свои документы |
| 40      | `management`       | Руководство: полный доступ к бизнес-данным ОП/ВО/ПО |
| 50      | `admin`            | Суперпользователь: полный доступ ко всему |
| 60      | `visa`             | Визовый отдел: лиды/клиенты/документы в своём отделе |
| 70      | `partner`          | Партнёрский отдел: лиды/клиенты (только свои) |
| 80      | `hr`               | Отдел кадров: пользователи + документы; без лидов/сделок |
| 90      | `legal`            | Юридический: клиенты + пользователи + документы; без лидов/сделок |

> Legacy коды (`audit` → `quality_control`, `leadership` / `manager` → `management`,
> `admin_staff` / `system_admin` → `admin`, `control` → `quality_control`) нормализуются
> в `authz.NormalizeRoleCode`.

## Матрица прав по действиям

### Лиды и сделки

| Действие              | sales | visa | partner | quality_control | management | hr | legal | admin |
|-----------------------|:-----:|:----:|:-------:|:---------------:|:----------:|:--:|:-----:|:-----:|
| leads.view            | ✅ dept | ✅ dept | ✅ own | ✅ all (RO) | ✅ all | ❌ | ❌ | ✅ all |
| leads.create          | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ | ✅ |
| leads.update          | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ | ✅ |
| leads.delete          | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| leads.transfer_manager| ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ✅ |
| leads.move_between_funnels | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ✅ |
| deals.view            | ✅ dept | ❌ | ❌ | ✅ all (RO) | ✅ all | ❌ | ❌ | ✅ |
| deals.create          | ✅ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ✅ |
| deals.update          | ✅ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ✅ |
| deals.delete          | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |

### Клиенты

| Действие         | sales | visa | partner | quality_control | management | hr | legal | admin |
|------------------|:-----:|:----:|:-------:|:---------------:|:----------:|:--:|:-----:|:-----:|
| clients.view     | ✅ all | ✅ all | ✅ all | ✅ all (RO) | ✅ all | ❌ | ✅ all | ✅ |
| clients.create   | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ | ✅ |
| clients.update   | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ | ✅ |
| clients.delete   | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| clients.export   | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |

> Область видимости клиентов: все роли с доступом видят **общую базу** (`ScopeKindAll`).
> Исключение: `partner` — только свои клиенты (`ScopeKindOwn`) через сервисный слой.

### Документы

| Действие              | sales | visa | partner | quality_control         | management | hr   | legal | admin |
|-----------------------|:-----:|:----:|:-------:|:-----------------------:|:----------:|:----:|:-----:|:-----:|
| documents.view        | ✅ dept | ✅ dept | ✅ dept | ✅ all depts (RO) | ✅ related | ✅ dept | ✅ dept | ✅ |
| documents.create      | ❌ | ❌ | ❌ | ✅ dept (свой отдел) | ✅ related | ✅ dept | ✅ dept | ✅ |
| documents.update      | ❌ | ❌ | ❌ | ✅ dept | ✅ related | ✅ dept | ✅ dept | ✅ |
| documents.send        | ✅ dept | ✅ dept | ❌ | ✅ dept | ✅ related | ✅ dept | ✅ dept | ✅ |
| documents.download    | ❌ | ❌ | ❌ | ✅ **dept** (только свой отдел) | ❌ | ✅ dept | ✅ dept | ✅ |
| documents.delete      | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |

> **quality_control**: `documents.download` ограничен `ScopeDepartment` (только свои документы),
> в отличие от `documents.view` который имеет `ScopeRelatedDepartments`.

### Пользователи

| Действие         | sales | visa | partner | quality_control | management | hr | legal | admin |
|------------------|:-----:|:----:|:-------:|:---------------:|:----------:|:--:|:-----:|:-----:|
| users.view       | ❌ | ❌ | ❌ | ❌ | ✅ related | ✅ dept | ✅ dept | ✅ |
| users.create     | ❌ | ❌ | ❌ | ❌ | ✅ | ✅ | ✅ | ✅ |
| users.update     | ❌ | ❌ | ❌ | ❌ | ✅ | ✅ | ✅ | ✅ |
| users.delete     | ❌ | ❌ | ❌ | ❌ | ✅ | ✅ | ✅ | ✅ |
| users.block      | ❌ | ❌ | ❌ | ❌ | ✅ | ✅ | ✅ | ✅ |

### Остальное

| Действие         | sales | visa | partner | quality_control | management | hr | legal | admin |
|------------------|:-----:|:----:|:-------:|:---------------:|:----------:|:--:|:-----:|:-----:|
| reports.view     | ❌ | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ |
| chat.view        | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| chat.delete      | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| messenger.view   | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ |
| telephony.view   | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| funnels.view     | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ |
| branches.view    | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ✅ |
| approvals.create | ✅ | ✅ | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |

## Область видимости данных (DataScope)

Функции `resolveLeadScope`, `resolveClientScope`, `resolveDealScope` в `internal/services/scope.go`
вычисляют область видимости записей для каждой роли:

| Сущность | sales | visa | partner | quality_control | management | hr | legal |
|----------|-------|------|---------|-----------------|------------|----|-------|
| Лиды     | Branch+Dept | Branch+Dept | Own | All (RO) | All | ❌ | ❌ |
| Сделки   | Branch+Dept | Branch+Dept | ❌ | All (RO) | All | ❌ | ❌ |
| Клиенты  | All | All | All (own service-side) | All (RO) | All | ❌ | All |

> `ReadOnlyGuard` middleware блокирует небезопасные HTTP-методы для `quality_control`.

## Защита маршрутов

Все защищённые маршруты проверяются через `middleware.RequirePermission(action, resource)`.

| Группа маршрутов | Защита |
|-----------------|--------|
| `/leads/**`     | `leads.view` / `leads.create` / `leads.update` / `leads.delete` / `deals.create` |
| `/deals/**`     | `deals.view` / `deals.create` / `deals.update` / `deals.delete` |
| `/clients/**`   | `clients.view` / `clients.create` / `clients.update` / `clients.delete` |
| `/documents/**` | `documents.view` / `documents.create` / `documents.update` / `documents.delete` / `documents.send` / `documents.download` |
| `/users/**`     | `users.view` / `users.create` / `users.update` / `users.delete` |
| `/chats/**`     | `chat.view` (группа); `chat.delete` (удаление чата/сообщения) |
| `/reports/**`   | `reports.view` |
| `/branches/**`  | `branches.view` / `branches.create` / `branches.update` / `branches.delete` |
| `/roles/**`     | `RequireRoles(admin)` |
| `/integrations/wazzup/**` | `messenger.view` |
| `/api/v1/telephony/**`    | `telephony.view` |

## Этапы реализации

### Этап 2 (leads/deals lifecycle)
- `POST /leads/:id/archive`, `POST /leads/:id/unarchive`
- `POST /deals/:id/archive`, `POST /deals/:id/unarchive`
- Hard delete только для admin (`role_id=50`)
- `archive` query-параметр: `active` | `archived` | `all`

### Этап 3 (clients/documents lifecycle)
- `POST /clients/:id/archive`, `POST /clients/:id/unarchive`
- `POST /documents/:id/archive`, `POST /documents/:id/unarchive`
- Hard delete только для admin
- `archive` query-параметр на list endpoints

### Этап 4 (tasks lifecycle)
- `POST /tasks/:id/archive`, `POST /tasks/:id/unarchive`
- Hard delete только для admin
- `archive` query-параметр на `GET /tasks`

## Ключевые политики

- `CanManageSystem` — только `admin` (role_id=50)
- `CanAssignRoles` — только `admin`
- `CanAccessLogs` — только `admin`
- `CanHardDeleteBusinessEntity` — только `admin`
- `IsReadOnly` — `quality_control` (role_id=30): ReadOnlyGuard блокирует POST/PUT/PATCH/DELETE кроме разрешённых chat-эндпоинтов

## Branch RBAC

- Архитектура CRM: **одна компания**, филиалы (`branches`) = структурные подразделения.
- `users.branch_id` определяет рабочий филиал сотрудника.
- `GET /branches` — admin и management видят все; остальные — только свой.
- `POST/PUT/DELETE /branches` — только admin.
