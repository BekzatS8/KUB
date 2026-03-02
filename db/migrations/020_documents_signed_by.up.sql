ALTER TABLE documents
    DROP CONSTRAINT IF EXISTS documents_status_chk;

ALTER TABLE documents
    ADD COLUMN IF NOT EXISTS signed_by TEXT;

ALTER TABLE documents
    ADD CONSTRAINT documents_status_chk CHECK (
        status IN ('draft','under_review','approved','returned','signed','sent_for_signature')
    );
