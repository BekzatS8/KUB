-- 088_drive_share_access.up.sql
-- Уровень доступа в хранилище: view — смотреть и скачивать; edit — ещё и
-- работать внутри выданной папки (создавать, загружать, вставлять,
-- переименовывать, перемещать, удалять в корзину). Саму выданную папку и то,
-- что выше неё, получатель не меняет. Уже выданные доступы — edit: так их и
-- задумывали (личные папки менеджеров, папки отделов).
--
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE drive_shares ADD COLUMN IF NOT EXISTS access TEXT NOT NULL DEFAULT 'edit';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'drive_shares_access_check') THEN
        ALTER TABLE drive_shares ADD CONSTRAINT drive_shares_access_check CHECK (access IN ('view', 'edit'));
    END IF;
END $$;
