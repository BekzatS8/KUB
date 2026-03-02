ALTER TABLE documents
    DROP CONSTRAINT IF EXISTS documents_status_chk;

ALTER TABLE documents
    DROP COLUMN IF EXISTS signed_by;

ALTER TABLE documents
    ADD CONSTRAINT documents_status_chk CHECK (
        status IN ('draft','under_review','approved','returned','signed')
    );
