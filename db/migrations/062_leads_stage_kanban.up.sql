-- 062_leads_stage_kanban.up.sql
-- Leads live on the funnel kanban board (ТЗ 04.07.2026, п.1.1).
--
-- New inbound leads must show up as cards on the first stage ("Новая заявка")
-- of the sales funnel so managers pick them up from the board instead of the
-- flat leads list. Leads get a stage_id and move across stages like deals.
-- This migration is re-applied on every deploy, so every statement is idempotent.

ALTER TABLE leads ADD COLUMN IF NOT EXISTS stage_id INT REFERENCES funnel_stages(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS leads_funnel_stage_idx ON leads(funnel_id, stage_id);

-- Attach active unconverted leads without a funnel to the default sales funnel
-- so the existing backlog becomes visible on the board.
UPDATE leads
SET funnel_id = (SELECT id FROM funnels WHERE code = 'sales_default')
WHERE funnel_id IS NULL
  AND is_archived = FALSE
  AND COALESCE(status, 'new') NOT IN ('converted', 'cancelled')
  AND EXISTS (SELECT 1 FROM funnels WHERE code = 'sales_default');

-- Put leads that have a funnel but no stage on the first active stage of
-- their funnel.
UPDATE leads l
SET stage_id = (
    SELECT fs.id
    FROM funnel_stages fs
    WHERE fs.funnel_id = l.funnel_id AND fs.is_active = TRUE
    ORDER BY fs.position ASC, fs.id ASC
    LIMIT 1
)
WHERE l.stage_id IS NULL
  AND l.funnel_id IS NOT NULL
  AND l.is_archived = FALSE
  AND COALESCE(l.status, 'new') NOT IN ('converted', 'cancelled');
