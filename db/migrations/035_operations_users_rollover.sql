-- =============================================================================
-- PRODUCTION SAFETY CHECK: users with legacy operations role_id=20
-- Run this BEFORE deploying migration 034 to production.
-- DO NOT execute the UPDATE automatically — verify the count and target role first.
-- =============================================================================

-- Step 1: Check how many users still have role_id=20
SELECT COUNT(*) AS operations_user_count
FROM users
WHERE role_id = 20;

-- Step 2: List them for manual review
SELECT id, email, first_name, last_name, role_id, branch_id, is_active
FROM users
WHERE role_id = 20
ORDER BY id;

-- =============================================================================
-- Step 3: Reassign to appropriate active role.
-- Choose ONE of the options below based on what operations staff should become:
-- =============================================================================

-- OPTION A: Reassign to sales (role_id=10) — typical for operations who work with leads
-- UPDATE users SET role_id = 10 WHERE role_id = 20;

-- OPTION B: Reassign to visa (role_id=60)
-- UPDATE users SET role_id = 60 WHERE role_id = 20;

-- OPTION C: Reassign per-user individually (safest, use after Step 2 review)
-- UPDATE users SET role_id = <target_role_id> WHERE id = <user_id>;

-- =============================================================================
-- Step 4: Verify no operations users remain
-- Expected: 0 rows
-- =============================================================================
-- SELECT id, email, role_id FROM users WHERE role_id = 20;
