# Backend production release checklist

Use this checklist for backend-only production releases.

## 1. Pre-release freeze

- Confirm the working tree contains only release changes.
- Run `go test ./...`.
- Confirm `.env.prod` and `config/config.yaml` contain production values and no placeholder secrets.
- Confirm production users with roles `sales`, `operations`, and `control` have a valid `branch_id`.

## 2. Backup

Create a database backup before migrations or container replacement:

```bash
mkdir -p backups
docker compose -f docker-compose.prod.yml exec -T postgres \
  pg_dump -U "${POSTGRES_USER:-turcompany}" -d "${POSTGRES_DB:-turcompany}" -Fc \
  > "backups/kub_pre_release_$(date +%Y%m%d_%H%M%S).dump"
```

Keep the backup outside git and confirm the file is non-empty.

## 3. Migrations and branch readiness

Apply migrations:

```bash
make migrate
```

Run branch readiness audit:

```bash
make branch-audit
```

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/run-branch-readiness-audit.ps1
```

Release rule:

- Any `CRITICAL` row with count `> 0` blocks the deploy.
- Safe backfills are allowed only when branch can be derived from linked data.
- Do not guess branch for unresolved users or business records; assign manually after business approval.

## 4. Deploy

```bash
make preflight
make up
```

If `make preflight` already started the stack successfully, `make up` is optional unless a rebuild/restart is required.

## 5. Post-deploy smoke

Run these checks immediately after deploy:

- `GET /healthz` returns `200`.
- `POST /auth/login` works for smoke users.
- `GET /users/me` returns `role` and `branch`.
- `GET /branches` works for authenticated users.
- A `sales` user from Branch A cannot see Branch B clients/leads/deals/documents/tasks/chats/reports.
- `operations` and `control` are limited to their own branch; `control` remains read-only for business writes.
- `leadership` and `system_admin` can see global business data and can use `branch_id` list/report filters.
- Direct document file/download URLs for foreign branch records return `403` or policy `404`.
- Task assignment to a scoped user from another branch is rejected.
- Chat directory/status/search do not reveal foreign-branch users to scoped roles.
- Reports ignore a foreign `branch_id` for scoped roles.

## 6. Monitoring

Watch logs and metrics for at least 30 minutes:

- unexpected `500` errors;
- sudden spike of `403` on branch-protected endpoints;
- migration or DB constraint errors;
- document file access failures;
- chat/task/report regressions.

## 7. Rollback

- If migrations fail: stop deploy and restore the backup if partial schema/data changes cannot be safely corrected.
- If the app does not start: roll back the backend image/commit; keep backward-compatible schema changes in place unless they caused the failure.
- If an access leak is found: temporarily block the affected route at proxy/WAF level or roll back the app, then patch branch scope.
- If unresolved branch data blocks users: assign branch manually through admin flow or approved SQL, then rerun `make branch-audit`.
