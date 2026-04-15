BEGIN;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS active_company_id INT REFERENCES companies(id);

CREATE INDEX IF NOT EXISTS users_active_company_idx ON users(active_company_id);

UPDATE users u
SET active_company_id = uc.company_id
FROM user_companies uc
WHERE uc.user_id = u.id
  AND uc.is_primary = TRUE
  AND uc.is_active = TRUE
  AND u.active_company_id IS NULL;

COMMIT;
