ALTER TABLE drive_shares DROP CONSTRAINT IF EXISTS drive_shares_access_check;
ALTER TABLE drive_shares DROP COLUMN IF EXISTS access;
