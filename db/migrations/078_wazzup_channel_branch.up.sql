BEGIN;

-- =============================================================================
-- Разделение лидов/клиентов по филиалам (обратная связь заказчика 22.08.2026).
--
-- В Wazzup каналы уже разделены по номерам = по филиалам (напр. Алматы/Шымкент).
-- Чтобы это отражалось в CRM, каждому каналу задаётся branch_id, и входящий лид
-- получает филиал своего канала. Менеджеры (sales/visa/partner) видят лиды и
-- клиентов ТОЛЬКО своего филиала; админ/руководство/контроль — все филиалы.
-- =============================================================================

-- Канал Wazzup ↔ филиал (nullable: пока админ не задал — филиал не определён).
ALTER TABLE wazzup_channels
    ADD COLUMN IF NOT EXISTS branch_id INT REFERENCES branches(id);

CREATE INDEX IF NOT EXISTS wazzup_channels_branch_id_idx ON wazzup_channels(branch_id);

-- Бэкфилл существующих лидов без филиала: берём филиал владельца. Иначе после
-- включения branch-scope такие лиды пропадут у всех менеджеров (fail-closed).
-- Идемпотентно: обновляются только строки с branch_id IS NULL.
UPDATE leads l
SET branch_id = u.branch_id
FROM users u
WHERE l.owner_id = u.id
  AND l.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

-- То же для клиентов.
UPDATE clients c
SET branch_id = u.branch_id
FROM users u
WHERE c.owner_id = u.id
  AND c.branch_id IS NULL
  AND u.branch_id IS NOT NULL;

COMMIT;
