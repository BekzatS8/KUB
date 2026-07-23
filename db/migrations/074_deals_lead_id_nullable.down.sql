-- Возврат NOT NULL возможен только если нет сделок без лида; иначе упадёт.
BEGIN;

ALTER TABLE IF EXISTS deals ALTER COLUMN lead_id SET NOT NULL;

COMMIT;
