-- 066_deals_prepayment.up.sql
-- Первоначальный взнос в сделке (ТЗ 04.07.2026, п.2.5).
--
-- Договор 500 000, клиент оплатил 250 000 → остаток 250 000. Остаток считается
-- как amount - prepayment; подставляется в договоры (PREPAY_AMOUNT_*,
-- DEAL_REMAIN_KZT_* плейсхолдеры).
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE deals ADD COLUMN IF NOT EXISTS prepayment NUMERIC(14,2) NOT NULL DEFAULT 0;
