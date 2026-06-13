BEGIN;

DROP TABLE IF EXISTS deal_stage_history;

ALTER TABLE deals DROP COLUMN IF EXISTS stage_id;

DROP TABLE IF EXISTS funnel_stages;

COMMIT;
