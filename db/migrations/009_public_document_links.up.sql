BEGIN;

CREATE TABLE IF NOT EXISTS public_document_links (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS public_document_links_token_hash_uidx ON public_document_links(token_hash);
CREATE INDEX IF NOT EXISTS public_document_links_expires_idx ON public_document_links(expires_at);
CREATE INDEX IF NOT EXISTS public_document_links_document_idx ON public_document_links(document_id);

COMMIT;
