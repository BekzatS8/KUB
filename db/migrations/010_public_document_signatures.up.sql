BEGIN;

CREATE TABLE IF NOT EXISTS public_document_signatures (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    link_id BIGINT NOT NULL REFERENCES public_document_links(id) ON DELETE CASCADE,
    signer_name TEXT NOT NULL,
    signer_email TEXT,
    signer_phone TEXT,
    signature TEXT NOT NULL,
    ip INET,
    user_agent TEXT,
    event_id UUID NOT NULL,
    signed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    meta JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS public_document_signatures_document_idx ON public_document_signatures(document_id);
CREATE INDEX IF NOT EXISTS public_document_signatures_link_idx ON public_document_signatures(link_id);
CREATE UNIQUE INDEX IF NOT EXISTS public_document_signatures_event_uidx ON public_document_signatures(event_id);

COMMIT;
