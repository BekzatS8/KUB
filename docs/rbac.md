# RBAC (каноническая модель)

## Source of truth

Единый источник ролей и policy-хелперов: `internal/authz/roles.go`.

## Роли и совместимость role_id

| role_id | Канонический code         | Legacy name | Назначение |
|--------:|---------------------------|-------------|------------|
| 10      | `sales`                   | `sales`     | Лиды/сделки/договоры в своей зоне |
| 15      | `backoffice_admin_staff`  | `staff`     | Административный персонал (задачи/мессенджер) |
| 20      | `operations`              | `operations`| Проверка и операционный документооборот |
| 30      | `control`                 | `audit`     | Глобальный read-only по бизнес-данным, без leadership данных |
| 40      | `leadership`              | `management`| Полный доступ к бизнес-данным |
| 50      | `system_admin`            | `admin`     | Суперпользователь: системное администрирование + полный доступ к бизнес-данным |

> Backward compatibility: исторический `RoleAdminStaff` оставлен как alias к `role_id=50`.

## Ключевые политики (текущий фундамент)

- `CanManageSystem` — только `system_admin`.
- `CanAssignRoles` — только `system_admin`.
- `CanAccessLogs` — только `system_admin`.
- `CanManageIntegrations` — только `system_admin`.
- `CanViewLeadershipData` — `leadership`, `system_admin`.
- `CanViewAllBusinessData` — `leadership`, `control`, `operations` (legacy-поведение сохранено).
- `CanAccessAllBusinessDataIncludingAdmin` — `leadership`, `control`, `operations`, `system_admin`.
- `CanArchiveBusinessEntity` — бизнес-роли, кроме read-only, плюс `system_admin`.
- `CanHardDeleteBusinessEntity` — только `system_admin` (`role_id=50`).

## Целевая модель (поэтапный переход)

- Для бизнес-сущностей (`leads`, `deals`, `clients`, `documents`, `tasks`) целевое действие удаления — **archive**, а не hard delete.
- **Hard delete бизнес-сущностей разрешён только `role_id=50`.**
- Бизнес-роли работают через archive-модель в рамках своих прав.
- Archive endpoints и полный перевод handlers/services на новую модель будут подключены **на следующем этапе**.

## Важные границы этапа

- На текущем этапе archive-модель касается только бизнес-сущностей.
- `users` и `roles` не входят в archive-модель этого этапа.
