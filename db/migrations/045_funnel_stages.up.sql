BEGIN;

-- =============================================================================
-- Step 1: funnel_stages table (idempotent: IF NOT EXISTS)
-- =============================================================================
CREATE TABLE IF NOT EXISTS funnel_stages (
    id          SERIAL PRIMARY KEY,
    funnel_id   INT NOT NULL REFERENCES funnels(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    code        VARCHAR(64) NOT NULL,
    color       VARCHAR(16) NOT NULL DEFAULT '#94a3b8',
    type        VARCHAR(16) NOT NULL DEFAULT 'regular' CHECK (type IN ('regular','won','lost')),
    position    INT NOT NULL DEFAULT 0,
    probability INT NOT NULL DEFAULT 0,
    description TEXT,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (funnel_id, code)
);

CREATE INDEX IF NOT EXISTS funnel_stages_funnel_id_idx ON funnel_stages(funnel_id);

-- =============================================================================
-- Step 2: deals.stage_id (idempotent: IF NOT EXISTS)
-- =============================================================================
ALTER TABLE deals
    ADD COLUMN IF NOT EXISTS stage_id INT REFERENCES funnel_stages(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS deals_stage_id_idx ON deals(stage_id);

-- =============================================================================
-- Step 3: deal_stage_history table (idempotent: IF NOT EXISTS)
-- =============================================================================
CREATE TABLE IF NOT EXISTS deal_stage_history (
    id            SERIAL PRIMARY KEY,
    deal_id       INT NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
    from_stage_id INT REFERENCES funnel_stages(id) ON DELETE SET NULL,
    to_stage_id   INT REFERENCES funnel_stages(id) ON DELETE SET NULL,
    changed_by    INT REFERENCES users(id) ON DELETE SET NULL,
    comment       TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS deal_stage_history_deal_id_idx ON deal_stage_history(deal_id);

-- =============================================================================
-- Step 4: Seed default stages for every existing funnel (idempotent: ON CONFLICT DO NOTHING)
-- Codes are aligned with deals.status values so deals can be backfilled below.
-- =============================================================================
-- Сидируем дефолтные этапы ТОЛЬКО для воронок без единого этапа.
-- Так повторный запуск не восстанавливает этапы, удалённые администратором.
INSERT INTO funnel_stages (funnel_id, name, code, color, type, position, probability)
SELECT f.id, s.name, s.code, s.color, s.type, s.position, s.probability
FROM funnels f
CROSS JOIN (VALUES
    ('Новая заявка',   'new',         '#94a3b8', 'regular', 10, 10),
    ('В работе',       'in_progress', '#3b82f6', 'regular', 20, 30),
    ('Переговоры',     'negotiation', '#f59e0b', 'regular', 30, 60),
    ('Успешно',        'won',         '#22c55e', 'won',     40, 100),
    ('Не реализовано', 'lost',        '#ef4444', 'lost',    50, 0)
) AS s(name, code, color, type, position, probability)
WHERE NOT EXISTS (
    SELECT 1 FROM funnel_stages fs WHERE fs.funnel_id = f.id
)
ON CONFLICT (funnel_id, code) DO NOTHING;

-- =============================================================================
-- Step 5: Backfill deals.stage_id from deals.status (idempotent: only NULL rows)
-- 'cancelled' deals are mapped onto the 'lost' stage of their funnel.
-- Deals without funnel_id are left without a stage (not shown on the kanban board).
-- =============================================================================
UPDATE deals d
SET stage_id = fs.id
FROM funnel_stages fs
WHERE fs.funnel_id = d.funnel_id
  AND fs.code = CASE WHEN COALESCE(d.status, 'new') = 'cancelled' THEN 'lost' ELSE COALESCE(d.status, 'new') END
  AND d.stage_id IS NULL;

COMMIT;
