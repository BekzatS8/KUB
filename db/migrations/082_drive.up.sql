-- 082_drive.up.sql
-- Хранилище файлов (раздел «Хранилище»): дерево папок и файлов + доступы.
--
-- Содержимое файлов лежит в объектном хранилище (S3), в базе — только
-- метаданные и ключ объекта. Доступ к узлу наследуется вниз по дереву: доступ
-- к папке открывает всё её содержимое.
--
-- This migration is re-applied on every deploy, so every statement is idempotent.

CREATE TABLE IF NOT EXISTS drive_nodes (
    id           BIGSERIAL PRIMARY KEY,
    -- NULL — корень хранилища. Удаление папки каскадом удаляет содержимое
    -- (объекты в S3 сервис удаляет сам, собрав ключи заранее).
    parent_id    BIGINT REFERENCES drive_nodes(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('folder', 'file')),
    name         TEXT NOT NULL CHECK (btrim(name) <> ''),
    storage_key  TEXT,
    size_bytes   BIGINT NOT NULL DEFAULT 0,
    mime_type    TEXT NOT NULL DEFAULT '',
    created_by   INT REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT drive_nodes_file_has_key CHECK (kind = 'folder' OR storage_key IS NOT NULL)
);

-- Имена уникальны внутри папки без учёта регистра (как в файловых системах
-- Windows/macOS). У корня parent_id = NULL, поэтому сворачиваем его в 0.
CREATE UNIQUE INDEX IF NOT EXISTS drive_nodes_name_uniq
    ON drive_nodes (COALESCE(parent_id, 0), lower(name));

CREATE INDEX IF NOT EXISTS drive_nodes_parent_idx ON drive_nodes (parent_id);

-- Доступ пользователя к файлу или папке. expires_at = NULL — бессрочно.
-- Один пользователь — одна запись на узел: повторная выдача обновляет срок.
CREATE TABLE IF NOT EXISTS drive_shares (
    id          BIGSERIAL PRIMARY KEY,
    node_id     BIGINT NOT NULL REFERENCES drive_nodes(id) ON DELETE CASCADE,
    user_id     INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ,
    created_by  INT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (node_id, user_id)
);

CREATE INDEX IF NOT EXISTS drive_shares_user_idx ON drive_shares (user_id);
