-- =============================================================================
-- DOCUMENTATION / POST-DEPLOY CHECKLIST
-- Role operations / role_id=20 — REMOVED
-- =============================================================================
--
-- CONTEXT:
--   role_id=20 / code='operations' was a legacy role that has been removed.
--   Migration 034 handles the full cleanup:
--     1. Clears code='operations' from any row in roles.
--     2. Migrates any remaining users with role_id=20 → role_id=60 (visa) before
--        deleting the role row (idempotent; no-op if no such users exist).
--     3. Deletes role_permissions WHERE role_id = 20.
--     4. Deletes the role row WHERE id = 20 OR code = 'operations'.
--
-- ACTIVE ROLES AFTER MIGRATION 034:
--   id=10  code='sales'
--   id=30  code='quality_control'
--   id=40  code='management'
--   id=50  code='admin'
--   id=60  code='visa'
--   id=70  code='partner'
--   id=80  code='hr'
--   id=90  code='legal'
--
-- =============================================================================
-- POST-DEPLOY VERIFICATION QUERIES (run after migration 034)
-- =============================================================================

-- Expected: 0 rows
SELECT COUNT(*) AS users_with_legacy_role
FROM users
WHERE role_id = 20;

-- Expected: 0 rows
SELECT *
FROM roles
WHERE id = 20 OR code = 'operations';

-- Expected: exactly 8 rows (ids 10,30,40,50,60,70,80,90)
SELECT id, name, description, code
FROM roles
ORDER BY id;
