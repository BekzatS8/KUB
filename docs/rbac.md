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
| 50      | `system_admin`            | `admin`     | Системное администрирование (роли/логи/интеграции) |

> Backward compatibility: исторический `RoleAdminStaff` оставлен как alias к `role_id=50`.

## Ключевые политики

- `CanManageSystem` — только `system_admin`.
- `CanAssignRoles` — только `system_admin`.
- `CanAccessLogs` — только `system_admin`.
- `CanManageIntegrations` — только `system_admin`.
- `CanViewLeadershipData` — `leadership`, `system_admin`.
- `CanViewAllBusinessData` — `leadership`, `control`, `operations`.
- `CanProcessDocuments` — `operations`, `leadership`.
- `CanWorkWithLeads` — `sales`, `operations`, `leadership`.
- `CanAccessMessengerOnly` — `backoffice_admin_staff`.

## Важное разграничение

- `leadership` и `system_admin` — **разные смысловые роли**.
- `leadership` управляет бизнес-данными.
- `system_admin` управляет системой и доступами, но не получает автоматически права владельца бизнес-сущностей.
