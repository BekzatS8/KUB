-- =====================================================================
-- KUB — fresh init schema (clean create, no ALTERs)
-- PostgreSQL
-- =====================================================================

BEGIN;

-- ===================== ROLES =====================
CREATE TABLE IF NOT EXISTS roles (
                                     id          SERIAL PRIMARY KEY,
                                     name        VARCHAR(255) NOT NULL,
                                     description TEXT
);

-- ===================== USERS =====================
CREATE TABLE IF NOT EXISTS users (
                                     id                   SERIAL PRIMARY KEY,
                                     company_name         VARCHAR(255),
                                     bin_iin              VARCHAR(255),
                                     email                VARCHAR(255) NOT NULL UNIQUE,
                                     password_hash        VARCHAR(255) NOT NULL,
                                     role_id              INT REFERENCES roles(id),

    -- refresh storage
                                     refresh_token        TEXT,
                                     refresh_expires_at   TIMESTAMPTZ,
                                     refresh_revoked      BOOLEAN NOT NULL DEFAULT FALSE,

    -- phone verification
                                     phone                VARCHAR(20),
                                     is_verified          BOOLEAN NOT NULL DEFAULT FALSE,
                                     verified_at          TIMESTAMPTZ,

    -- telegram
                                     telegram_chat_id     BIGINT UNIQUE,
                                     notify_tasks_telegram BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE UNIQUE INDEX IF NOT EXISTS users_refresh_token_uq
    ON users (refresh_token)
    WHERE refresh_token IS NOT NULL;

CREATE INDEX IF NOT EXISTS users_role_idx         ON users(role_id);
CREATE INDEX IF NOT EXISTS users_is_verified_idx  ON users(is_verified);

-- ===================== LEADS =====================
CREATE TABLE IF NOT EXISTS leads (
                                     id          SERIAL PRIMARY KEY,
                                     title       VARCHAR(255) NOT NULL,
                                     description TEXT,
                                     owner_id    INT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
                                     status      VARCHAR(100),
                                     created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS leads_owner_idx   ON leads(owner_id);
CREATE INDEX IF NOT EXISTS leads_status_idx  ON leads(status);

-- ===================== DEALS =====================
CREATE TABLE IF NOT EXISTS deals (
                                     id         SERIAL PRIMARY KEY,
                                     lead_id    INT REFERENCES leads(id) ON DELETE SET NULL,
                                     owner_id   INT REFERENCES users(id) ON DELETE SET NULL,
                                     amount     VARCHAR(20) NOT NULL,
                                     currency   VARCHAR(10) NOT NULL,
                                     status     VARCHAR(100),
                                     created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                     CONSTRAINT deals_status_chk CHECK (status IN ('new','in_progress','negotiation','won','lost','cancelled'))
);

CREATE INDEX IF NOT EXISTS deals_lead_idx    ON deals(lead_id);
CREATE INDEX IF NOT EXISTS deals_owner_idx   ON deals(owner_id);
CREATE INDEX IF NOT EXISTS deals_status_idx  ON deals(status);

-- ===================== DOCUMENTS =====================
CREATE TABLE IF NOT EXISTS documents (
                                         id        SERIAL PRIMARY KEY,
                                         deal_id   INT REFERENCES deals(id) ON DELETE CASCADE,
                                         doc_type  VARCHAR(100),
                                         file_path VARCHAR(255),
                                         status    VARCHAR(100),
                                         signed_at TIMESTAMPTZ,
                                         CONSTRAINT documents_status_chk CHECK (status IN ('draft','under_review','approved','returned','signed'))
);

CREATE INDEX IF NOT EXISTS documents_deal_idx   ON documents(deal_id);
CREATE INDEX IF NOT EXISTS documents_status_idx ON documents(status);

-- ===================== TASKS =====================
CREATE TABLE IF NOT EXISTS tasks (
                                     id               SERIAL PRIMARY KEY,
                                     creator_id       INT REFERENCES users(id) ON DELETE SET NULL,
                                     assignee_id      INT REFERENCES users(id) ON DELETE SET NULL,
                                     entity_id        INT NOT NULL,
                                     entity_type      VARCHAR(100) NOT NULL,                        -- 'deal' | 'lead'
                                     title            TEXT NOT NULL,
                                     description      TEXT,
                                     due_date         TIMESTAMPTZ,

                                     priority         VARCHAR(20)  NOT NULL DEFAULT 'normal',       -- low|normal|high|urgent
                                     status           VARCHAR(100) NOT NULL DEFAULT 'new',          -- new|in_progress|done|cancelled
                                     reminder_at      TIMESTAMPTZ,
                                     last_reminded_at TIMESTAMPTZ,
                                     created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                     updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

                                     CONSTRAINT tasks_entity_type_chk CHECK (entity_type IN ('deal','lead')),
                                     CONSTRAINT tasks_status_chk      CHECK (status IN ('new','in_progress','done','cancelled')),
                                     CONSTRAINT tasks_priority_chk    CHECK (priority IN ('low','normal','high','urgent'))
);

CREATE INDEX IF NOT EXISTS tasks_creator_idx   ON tasks(creator_id);
CREATE INDEX IF NOT EXISTS tasks_assignee_idx  ON tasks(assignee_id);
CREATE INDEX IF NOT EXISTS tasks_entity_idx    ON tasks(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS tasks_status_idx    ON tasks(status);
CREATE INDEX IF NOT EXISTS tasks_priority_idx  ON tasks(priority);
CREATE INDEX IF NOT EXISTS tasks_reminder_idx  ON tasks(reminder_at) WHERE reminder_at IS NOT NULL;

-- ===================== SMS (for documents) =====================
CREATE TABLE IF NOT EXISTS sms_confirmations (
                                                 id            SERIAL PRIMARY KEY,
                                                 document_id   INT REFERENCES documents(id) ON DELETE CASCADE,
                                                 sms_code      VARCHAR(100),
                                                 sent_at       TIMESTAMPTZ,
                                                 confirmed     BOOLEAN DEFAULT FALSE,
                                                 confirmed_at  TIMESTAMPTZ,
                                                 phone         VARCHAR(20)
);

CREATE INDEX IF NOT EXISTS sms_document_idx   ON sms_confirmations(document_id);
CREATE INDEX IF NOT EXISTS sms_confirmed_idx  ON sms_confirmations(confirmed);

-- ===================== USER VERIFICATIONS (for registration) =========
CREATE TABLE IF NOT EXISTS user_verifications (
                                                  id             SERIAL PRIMARY KEY,
                                                  user_id        INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                                                  code_hash      TEXT        NOT NULL,
                                                  sent_at        TIMESTAMPTZ NOT NULL,
                                                  expires_at     TIMESTAMPTZ NOT NULL,
                                                  confirmed      BOOLEAN     NOT NULL DEFAULT FALSE,
                                                  attempts       INT         NOT NULL DEFAULT 0,
                                                  confirmed_at   TIMESTAMPTZ,
                                                  last_resend_at TIMESTAMPTZ,
                                                  resend_count   INT         NOT NULL DEFAULT 0,
                                                  CONSTRAINT user_verifications_attempts_chk     CHECK (attempts >= 0),
                                                  CONSTRAINT user_verifications_resend_count_chk CHECK (resend_count >= 0)
);

CREATE INDEX IF NOT EXISTS user_verif_user_sent_idx
    ON user_verifications(user_id, sent_at DESC);
CREATE INDEX IF NOT EXISTS user_verif_confirmed_idx
    ON user_verifications(confirmed);

-- ===================== TELEGRAM LINKS (one-time codes) ================
CREATE TABLE IF NOT EXISTS telegram_links (
                                              id         SERIAL PRIMARY KEY,
                                              user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                                              code       VARCHAR(64) NOT NULL UNIQUE,
                                              expires_at TIMESTAMPTZ NOT NULL,
                                              used       BOOLEAN NOT NULL DEFAULT FALSE,
                                              created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS telegram_links_user_idx   ON telegram_links(user_id);
CREATE INDEX IF NOT EXISTS telegram_links_used_idx   ON telegram_links(used);
CREATE INDEX IF NOT EXISTS telegram_links_exp_idx    ON telegram_links(expires_at);

-- ===================== SEED ROLES (NO STAFF) =====================
INSERT INTO roles (id, name, description) VALUES
                                              (10,'sales','Продажник: лиды/сделки/черновики документов.'),
                                              (20,'operations','Операционный отдел: проверка документов.'),
                                              (30,'audit','Контроль: read-only ко всем данным.'),
                                              (40,'management','Руководство: расширенный доступ.'),
                                              (50,'admin','Администратор: управление системой.')
ON CONFLICT (id) DO NOTHING;

COMMIT;

-- =====================================================================
-- (опционально) пример вставки администратора — подставь свой bcrypt:
-- INSERT INTO users (company_name, bin_iin, email, password_hash, role_id, is_verified)
-- VALUES ('Admin', '000000000000', 'admin@example.com', '$2y$12$<bcrypt_here>', 50, TRUE);
-- =====================================================================
