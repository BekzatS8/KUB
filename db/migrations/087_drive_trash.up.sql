-- 087_drive_trash.up.sql
-- Корзина хранилища. «Удалить» переносит элемент в корзину: он и всё его
-- содержимое помечаются deleted_at и пропадают из списков, путей, доступов и
-- ссылок. trash_root_id — элемент, который удаляли (корень записи в корзине):
-- «Восстановить» возвращает всё с этим trash_root_id. «Удалить навсегда» и
-- «Очистить корзину» стирают записи и объекты в хранилище.
--
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE drive_nodes ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE drive_nodes ADD COLUMN IF NOT EXISTS deleted_by INT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE drive_nodes ADD COLUMN IF NOT EXISTS trash_root_id BIGINT;

CREATE INDEX IF NOT EXISTS drive_nodes_trash_root_idx ON drive_nodes (trash_root_id) WHERE trash_root_id IS NOT NULL;

-- Имена уникальны внутри папки без учёта регистра (как в Windows/macOS), но
-- только среди неудалённых: имя файла в корзине можно занять новым.
DROP INDEX IF EXISTS drive_nodes_name_uniq;
CREATE UNIQUE INDEX IF NOT EXISTS drive_nodes_live_name_uniq
    ON drive_nodes (COALESCE(parent_id, 0), lower(name))
    WHERE deleted_at IS NULL;
