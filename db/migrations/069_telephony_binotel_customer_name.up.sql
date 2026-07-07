-- 069_telephony_binotel_customer_name.up.sql
-- Имя клиента, сохранённое в адресной книге Binotel (customerData.name), чтобы в
-- разделе «Телефония» было видно, кто звонил, даже если номер не привязан к
-- клиенту/лиду в CRM.
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE telephony_calls ADD COLUMN IF NOT EXISTS binotel_customer_name TEXT;
