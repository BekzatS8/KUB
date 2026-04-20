# Branch Isolation — Production Rollout Package

Last updated: 2026-04-20

## 1) Release checklist

### 1.1 Database migrations
- [ ] Confirm migration `030_clients_branch_scope.up.sql` is applied in staging and production.
- [ ] Confirm down migration exists and is reviewed (`030_clients_branch_scope.down.sql`).
- [ ] Validate schema after migration:
  - [ ] `clients.branch_id` exists.
  - [ ] index on `clients.branch_id` exists.
  - [ ] expected FK/constraints are present (if enabled in environment).

### 1.2 Backfill verification
- [ ] Run backfill verification query and capture metrics:
  - [ ] total clients.
  - [ ] clients with non-null `branch_id`.
  - [ ] clients with null `branch_id`.
  - [ ] distribution by branch.
- [ ] Validate owner-derived backfill quality (sample random records from each branch).
- [ ] Define remediation set for null/invalid branch assignments before full rollout.

### 1.3 Foreign keys / indexes / query plans
- [ ] Verify FK/index health (no invalid indexes, no missing constraints).
- [ ] Review query plans for branch-scoped list/search/report queries on prod-like data.
- [ ] Confirm acceptable p95 for:
  - clients list/search,
  - leads/deals list/search,
  - documents list/search,
  - report/revenue aggregations.

### 1.4 App wiring
- [ ] Confirm `UserRepo` wiring in app bootstrap for services that resolve branch scope.
- [ ] Confirm `DocumentService` receives `UserRepo`.
- [ ] Confirm `ClientService` receives `UserRepo`.
- [ ] Confirm task/deal/lead services are wired with user context dependencies.
- [ ] Confirm no fallback service path bypasses branch scope resolution.

### 1.5 Role matrix verification
- [ ] Validate role IDs and permissions in current build:
  - Sales (10)
  - Operations (20)
  - Control (30, read-only)
  - Management (40)
  - System Admin (50)
- [ ] Validate branch visibility policy:
  - Sales/Operations/Control => own branch only.
  - Management/System Admin => global (plus optional branch filter).

### 1.6 Environment-specific configuration
- [ ] Confirm auth claims mapping to `user_id` + `role_id` works identically in staging/prod.
- [ ] Confirm user directory has valid branch assignments for active users.
- [ ] Confirm files/documents storage paths are correct per environment.
- [ ] Confirm timezone consistency for reports and archival timestamps.

### 1.7 Observability / logging
- [ ] Add or verify dashboards:
  - 403/404 by branch-protected endpoint group,
  - report/export response counts per role,
  - search result volume anomalies by role.
- [ ] Add alerts:
  - sudden spike in forbidden responses,
  - unexpected drop in report/export volume,
  - elevated latency for branch-filtered queries.
- [ ] Ensure logs capture role/user/endpoint + decision (allowed/forbidden) without sensitive payload leaks.

### 1.8 Feature flags (if used)
- [ ] Confirm default flag state in production.
- [ ] Confirm staged enablement plan.
- [ ] Confirm kill-switch and rollback owner.

---

## 2) QA matrix

### 2.1 Roles
- sales
- operations
- control
- management
- system_admin

### 2.2 Scenarios
- list
- get by id
- create
- update
- delete
- archive/unarchive
- change status
- assign
- search
- export
- documents/files/download
- analytics/reports

### 2.3 Expected outcomes by role and branch context

Legend:
- ✅ Allowed
- ❌ Denied
- N/A Not applicable by role/capability

| Scenario | Sales own | Sales foreign | Ops own | Ops foreign | Control own | Control foreign | Mgmt (global) | SysAdmin (global) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| List | ✅ | ❌ | ✅ | ❌ | ✅ (read-only) | ❌ | ✅ | ✅ |
| Get by ID | ✅ | ❌ | ✅ | ❌ | ✅ (read-only) | ❌ | ✅ | ✅ |
| Create | ✅ (role/ownership limits) | ❌ | ✅ | ❌ | N/A | N/A | ✅ | ✅ |
| Update | ✅ (role/ownership limits) | ❌ | ✅ | ❌ | N/A | N/A | ✅ | ✅ |
| Delete | N/A\* | N/A | N/A\* | N/A | N/A | N/A | N/A\* | ✅ |
| Archive/Unarchive | ✅ (if role permits) | ❌ | ✅ | ❌ | N/A (read-only) | N/A | ✅ | ✅ |
| Change Status | ✅ (role limits) | ❌ | ✅ | ❌ | N/A (read-only) | N/A | ✅ | ✅ |
| Assign | ✅ (self-only where required) | ❌ | ✅ | ❌ | N/A (read-only) | N/A | ✅ | ✅ |
| Search | ✅ | ❌ | ✅ | ❌ | ✅ (read-only) | ❌ | ✅ | ✅ |
| Export | ✅ (scoped) | ❌ | ✅ (scoped) | ❌ | ✅ (scoped/read-only) | ❌ | ✅ | ✅ |
| Documents/files/download | ✅ | ❌ | ✅ | ❌ | ✅ (view-only) | ❌ | ✅ | ✅ |
| Analytics/reports | ✅ (scoped) | ❌ | ✅ (scoped) | ❌ | ✅ (scoped) | ❌ | ✅ | ✅ |

\* Hard delete is expected to remain highly restricted (typically system_admin only).

### 2.4 Mandatory negative tests
- [ ] Attempt foreign `branch_id` in query params as scoped role.
- [ ] Attempt ID enumeration (neighbor IDs) for clients/leads/deals/documents/tasks.
- [ ] Attempt foreign document file download by direct URL.
- [ ] Attempt report/export with explicit foreign `branch_id`.
- [ ] Attempt search by BIN/IIN/name/doc number for foreign branch records.

---

## 3) Smoke tests (post-deploy)

1. Branch isolation basic:
   - [ ] User from branch A does not see branch B entities in list/search.
2. Admin/global visibility:
   - [ ] System admin sees branch A + B.
3. Create path branch assignment:
   - [ ] New client/lead/deal created by branch A user has branch A assignment.
4. Reports/export:
   - [ ] Scoped role report/export excludes foreign-branch data.
5. Documents/files:
   - [ ] Foreign branch document/file download is blocked.
6. Search:
   - [ ] Scoped role cannot discover foreign records via q/search/filter.

---

## 4) Rollout plan

### Phase 0 — Preflight (staging)
1. Apply migration in staging.
2. Run full QA matrix on staging with role-based test users across at least 2 branches.
3. Run performance sanity for list/search/report endpoints.

### Phase 1 — Production migration
1. Announce maintenance window / low-risk period.
2. Apply DB migration first.
3. Run backfill verification SQL and produce a signed-off report.

### Phase 2 — Application rollout
1. Deploy branch-aware code to production (canary first if available).
2. Validate app wiring and role matrix via smoke tests.
3. Enable feature flag (if present) progressively:
   - internal users,
   - one branch,
   - all users.

### Phase 3 — Stabilization
1. Monitor error-rate/latency/403 metrics for 24–48h.
2. Audit sampled audit logs for allow/deny correctness.
3. Capture post-rollout report for engineering + product.

### Handling existing records with `NULL branch_id`
1. Create remediation list:
   - records with owner having known branch,
   - records with missing owner branch,
   - orphan/legacy edge cases.
2. Auto-fix where owner branch is known.
3. Escalate unresolved rows for manual assignment with business owner.
4. Re-run verification and keep unresolved count at agreed threshold (ideally zero).

---

## 5) Rollback plan

### 5.1 App rollback (preferred first)
1. Disable feature flag (if used).
2. Roll back application deployment to last known stable release.
3. Keep schema changes in place unless they break backward compatibility.

### 5.2 Data rollback / remediation
1. If access leak is confirmed, immediately restrict vulnerable endpoints (WAF/routing/temporary deny).
2. Re-run branch assignment remediation for inconsistent records.
3. Re-validate with smoke + targeted QA tests.

### 5.3 DB rollback (last resort)
1. Execute down migration only if absolutely required.
2. Confirm impact on application compatibility before execution.
3. Communicate potential data-loss/semantic rollback effects to stakeholders.

---

## 6) Residual risks (post-release watchlist)

1. New endpoints may bypass scoped service path if implemented incorrectly.
2. New export paths may not inherit report/list branch policy by default.
3. Background jobs/integrations/ad-hoc scripts may bypass HTTP-level policy.
4. Misconfiguration in user-role-branch wiring can weaken isolation.
5. Legacy records may still have incomplete branch context without continued data governance.

---

## 7) Short team-facing summary

- Branch-based isolation is enforced across clients/leads/deals/documents/tasks, including ID-based and state-changing operations.
- Documents list/search and reports/analytics are branch-aware and respect role-based scope.
- Visibility model:
  - **Sales / Operations / Control**: own branch scope (Control is read-only).
  - **Management / System Admin**: global visibility (with optional branch filters).
- Remaining assumptions:
  - proper user-to-branch mapping,
  - no bypass via new non-scoped endpoints,
  - continuous checks for legacy records with missing branch metadata.

---

## Appendix A — Suggested verification SQL (template)

```sql
-- clients coverage
SELECT
  COUNT(*) AS total_clients,
  COUNT(*) FILTER (WHERE branch_id IS NOT NULL) AS clients_with_branch,
  COUNT(*) FILTER (WHERE branch_id IS NULL) AS clients_without_branch
FROM clients;

-- distribution
SELECT branch_id, COUNT(*) AS cnt
FROM clients
GROUP BY branch_id
ORDER BY cnt DESC;
```

## Appendix B — Suggested post-deploy dashboards
- Forbidden ratio by endpoint group (`/clients`, `/leads`, `/deals`, `/documents`, `/tasks`, `/reports`).
- Search result count percentile by role.
- Report/export response volume by role and branch filter usage.
