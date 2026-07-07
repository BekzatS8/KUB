-- 062_leads_stage_kanban.down.sql
DROP INDEX IF EXISTS leads_funnel_stage_idx;
ALTER TABLE leads DROP COLUMN IF EXISTS stage_id;
