# RBAC (каноническая модель)

## Source of truth

Единый источник ролей и policy-хелперов: `internal/authz/roles.go`.

## Роли и совместимость role_id

| role_id | Канонический code         | Legacy name | Назначение |
|--------:|---------------------------|-------------|------------|
| 10      | `sales`                   | `sales`     | Лиды/сделки/договоры в своей зоне |
| 20      | `operations`              | `operations`| Проверка и операционный документооборот |
| 30      | `control`                 | `audit`     | Глобальный read-only по бизнес-данным, без leadership данных |
| 40      | `leadership`              | `management`| Полный доступ к бизнес-данным |
| 50      | `system_admin`            | `admin`     | Суперпользователь: системное администрирование + полный доступ к бизнес-данным |

> Backward compatibility: исторический `RoleAdminStaff` оставлен как alias к `role_id=50`.

## Ключевые политики (текущий фундамент)

- `CanManageSystem` — только `system_admin`.
- `CanAssignRoles` — только `system_admin`.
- `CanAccessLogs` — только `system_admin`.
- `CanManageIntegrations` — любая аутентифицированная известная роль (`sales`, `operations`, `control`, `leadership`, `system_admin`); unknown role — denied.
- `CanViewLeadershipData` — `leadership`, `system_admin`.
- `CanViewAllBusinessData` — `leadership`, `control`, `operations` (legacy-поведение сохранено).
- `CanAccessAllBusinessDataIncludingAdmin` — `leadership`, `control`, `operations`, `system_admin`.
- `CanArchiveBusinessEntity` — бизнес-роли, кроме read-only, плюс `system_admin`.
- `CanHardDeleteBusinessEntity` — только `system_admin` (`role_id=50`).

## Этап 2 (реализовано для leads/deals)

- Для `leads` и `deals` системный администратор (`role_id=50`) имеет полный доступ как superuser (read/create/update/archive/unarchive/hard delete).
- Старый запрет вида «system admin cannot access business entity» для `leads`/`deals` снят.
- Для `leads`/`deals` hard delete (`DELETE /leads/:id`, `DELETE /deals/:id`) доступен только `role_id=50`.
- Для `leads`/`deals` archive/unarchive доступны бизнес-ролям с правом изменения и `role_id=50`.
- Read-only роль (`role_id=30`) не может archive/unarchive/hard delete.
- Добавлены явные endpoints:
  - `POST /leads/:id/archive`
  - `POST /leads/:id/unarchive`
  - `POST /deals/:id/archive`
  - `POST /deals/:id/unarchive`
- `DELETE` не превращается в archive: не-admin получают `403 Forbidden`.
- Для list в `leads`/`deals` добавлен query-параметр `archive`:
  - по умолчанию `active` (только активные),
  - `archive=archived` (только архив),
  - `archive=all` (все записи).

## Этап 3 (реализовано для clients/documents)

- Для `clients` и `documents` системный администратор (`role_id=50`) имеет полный доступ как superuser (read/create/update/archive/unarchive/hard delete).
- Старые запреты для system_admin на business entities сняты именно для `clients`/`documents`.
- `DELETE /clients/:id` и `DELETE /documents/:id` — только hard delete для `role_id=50`.
- Для остальных business-ролей удаление переведено на явные действия archive/unarchive:
  - `POST /clients/:id/archive`
  - `POST /clients/:id/unarchive`
  - `POST /documents/:id/archive`
  - `POST /documents/:id/unarchive`
- Read-only роль (`role_id=30`) не может archive/unarchive/hard delete.
- Для list endpoints поддержан `archive` filter:
  - `archive=active` (или пусто) — только активные,
  - `archive=archived` — только архивные,
  - `archive=all` — все.

## Границы после этапа 3

- `users` и `roles` по-прежнему вне archive scope.
- `tasks` остаются на следующий этап.

## Этап 4 (реализовано для tasks)

- Для `tasks` системный администратор (`role_id=50`) имеет полный доступ как superuser (read/create/update/archive/unarchive/hard delete).
- Убраны прежние self-only ограничения для `role_id=50`: это ограничение остаётся только для `sales` (`role_id=10`).
- `DELETE /tasks/:id` — только hard delete для `role_id=50`; остальным ролям возвращается `403 Forbidden`.
- Для soft lifecycle добавлены явные endpoints:
  - `POST /tasks/:id/archive`
  - `POST /tasks/:id/unarchive`
- Read-only роль (`role_id=30`) не может archive/unarchive/hard delete.
- Для list endpoint `GET /tasks` поддержан query-параметр `archive`:
  - `archive=active` (или пусто) — только активные,
  - `archive=archived` — только архивные,
  - `archive=all` — все.

## Границы после этапа 4

- `users` и `roles` по-прежнему вне archive scope.

## Multi-company ACL (финальная политика)

### `/companies`
- `GET /companies` — любой аутентифицированный пользователь, но только membership-scoped список (компании из `user_companies` текущего пользователя).
- `GET /companies/:id` — доступ только при membership в этой компании; иначе `404 company_not_found`.

### `/companies/:id/integrations`
- `GET/POST/PUT/DELETE` — только `leadership` (`role_id=40`) и `system_admin` (`role_id=50`).
- membership к `:id` обязателен.
- Для всех остальных ролей — `403 Forbidden`.
