-- 098_drive_passwords.up.sql
-- ТЗ «доработки CRM КУБ», п.8: пароль на файлы и папки хранилища (поставить,
-- сменить, удалить). Открыть защищённый узел и всё внутри него можно только
-- после ввода пароля. Хранится bcrypt-хеш. Идемпотентно.
ALTER TABLE drive_nodes
    ADD COLUMN IF NOT EXISTS password_hash   TEXT,
    ADD COLUMN IF NOT EXISTS password_set_by INT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS password_set_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS drive_nodes_protected_idx ON drive_nodes(id) WHERE password_hash IS NOT NULL;
