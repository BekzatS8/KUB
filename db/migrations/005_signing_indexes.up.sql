-- ===================== SIGNING INDEXES =====================

BEGIN;

CREATE INDEX IF NOT EXISTS signature_confirmations_token_hash_idx
    ON signature_confirmations(token_hash);
CREATE INDEX IF NOT EXISTS signature_confirmations_doc_status_idx
    ON signature_confirmations(document_id, status);
CREATE INDEX IF NOT EXISTS signature_confirmations_expires_idx
    ON signature_confirmations(expires_at);

CREATE INDEX IF NOT EXISTS sign_sessions_token_hash_idx
    ON sign_sessions(token_hash);
CREATE INDEX IF NOT EXISTS sign_sessions_doc_status_idx
    ON sign_sessions(document_id, status);
CREATE INDEX IF NOT EXISTS sign_sessions_expires_idx
    ON sign_sessions(expires_at);

COMMIT;
