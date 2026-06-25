BEGIN;

-- Revert: set NULL owner_id to first admin before re-adding NOT NULL.
UPDATE leads
SET owner_id = (
    SELECT id FROM users WHERE role_id = 10 ORDER BY id LIMIT 1
)
WHERE owner_id IS NULL;

ALTER TABLE leads ALTER COLUMN owner_id SET NOT NULL;

COMMIT;
