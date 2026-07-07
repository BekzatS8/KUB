-- 066_deals_prepayment.down.sql
ALTER TABLE deals DROP COLUMN IF EXISTS prepayment;
