CREATE TABLE funnel_transition_rules (
    id              SERIAL PRIMARY KEY,
    name            TEXT NOT NULL DEFAULT '',
    from_funnel_id  INT NOT NULL REFERENCES funnels(id) ON DELETE CASCADE,
    from_stage_id   INT NOT NULL REFERENCES funnel_stages(id) ON DELETE CASCADE,
    to_funnel_id    INT NOT NULL REFERENCES funnels(id) ON DELETE CASCADE,
    to_stage_id     INT NOT NULL REFERENCES funnel_stages(id) ON DELETE CASCADE,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_funnel_transition_rules UNIQUE (from_funnel_id, from_stage_id, to_funnel_id, to_stage_id)
);

CREATE INDEX idx_ftr_from ON funnel_transition_rules (from_funnel_id, from_stage_id)
    WHERE is_active = TRUE;
