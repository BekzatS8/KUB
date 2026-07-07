-- 067_soft_delete_trash.up.sql
-- Корзина: мягкое удаление бизнес-сущностей (ТЗ 04.07.2026, п.7.1).
--
-- «Везде обязательно нужна корзина, куда всё удалённое уходит»: удаление
-- лида/сделки/документа/клиента больше не стирает строку, а помечает её
-- deleted_at/deleted_by. Админ видит корзину (кто и что удалил), может
-- восстановить или удалить окончательно.
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE leads     ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE leads     ADD COLUMN IF NOT EXISTS deleted_by INT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE deals     ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE deals     ADD COLUMN IF NOT EXISTS deleted_by INT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS deleted_by INT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE clients   ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE clients   ADD COLUMN IF NOT EXISTS deleted_by INT REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS leads_deleted_idx     ON leads(deleted_at)     WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS deals_deleted_idx     ON deals(deleted_at)     WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS documents_deleted_idx ON documents(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS clients_deleted_idx   ON clients(deleted_at)   WHERE deleted_at IS NOT NULL;
