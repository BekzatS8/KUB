-- 064_users_internal_phone.up.sql
-- Привязка звонков Binotel к сотрудникам (ТЗ 04.07.2026, п.5.2).
--
-- Binotel передаёт внутренний номер (extension, например "113"), а не мобильный
-- телефон сотрудника, поэтому звонки не привязывались к менеджерам и раздел
-- «Телефония» у них показывал 0. users.internal_phone хранит внутренний номер
-- сотрудника из Binotel; FindManagerByExtension матчится по нему в первую очередь.
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE users ADD COLUMN IF NOT EXISTS internal_phone VARCHAR(32);

CREATE INDEX IF NOT EXISTS users_internal_phone_idx ON users(internal_phone) WHERE internal_phone IS NOT NULL;
