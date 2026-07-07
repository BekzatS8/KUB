-- 069_telephony_binotel_customer_name.down.sql

ALTER TABLE telephony_calls DROP COLUMN IF EXISTS binotel_customer_name;
