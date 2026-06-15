-- 047: Document versions table for version history
BEGIN;

CREATE TABLE IF NOT EXISTS document_versions (
    id            SERIAL PRIMARY KEY,
    document_id   INT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    version       INT NOT NULL,
    file_path     VARCHAR(255),
    file_path_pdf TEXT,
    file_path_docx TEXT,
    file_size     BIGINT,
    mime_type     VARCHAR(100),
    uploaded_by   INT REFERENCES users(id),
    comment       TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(document_id, version)
);

CREATE INDEX IF NOT EXISTS idx_document_versions_document_id ON document_versions(document_id);

COMMIT;
