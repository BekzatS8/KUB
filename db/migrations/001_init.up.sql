-- =====================================================================
-- KUB: base schema (roles, users, leads, deals, documents, messages,
--      tasks, sms_confirmations[docs], user_verifications[users])
-- PostgreSQL
-- =====================================================================

BEGIN;

-- ==== ROLES ===========================================================
CREATE TABLE IF NOT EXISTS roles (
                                     id          SERIAL PRIMARY KEY,
                                     name        VARCHAR(255) NOT NULL,
                                     description TEXT
);

-- ==== USERS ===========================================================
-- хранит refresh-токены и флаги верификации телефона
CREATE TABLE IF NOT EXISTS users (
                                     id                 SERIAL PRIMARY KEY,
                                     company_name       VARCHAR(255),
                                     bin_iin            VARCHAR(255),
                                     email              VARCHAR(255) NOT NULL UNIQUE,
                                     password_hash      VARCHAR(255) NOT NULL,
                                     role_id            INT REFERENCES roles(id),

    -- refresh storage
                                     refresh_token      TEXT,
                                     refresh_expires_at TIMESTAMPTZ,
                                     refresh_revoked    BOOLEAN NOT NULL DEFAULT FALSE,

    -- phone verification
                                     phone              VARCHAR(20),
                                     is_verified        BOOLEAN NOT NULL DEFAULT FALSE,
                                     verified_at        TIMESTAMPTZ
);

-- Апгрейд уже существующей таблицы (если поля отсутствуют)
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS refresh_token       TEXT,
    ADD COLUMN IF NOT EXISTS refresh_expires_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS refresh_revoked     BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS phone               VARCHAR(20),
    ADD COLUMN IF NOT EXISTS is_verified         BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS verified_at         TIMESTAMPTZ;

-- Уникальность только среди НЕ-NULL refresh_token
CREATE UNIQUE INDEX IF NOT EXISTS users_refresh_token_uq
    ON users (refresh_token)
    WHERE refresh_token IS NOT NULL;

-- Полезные индексы
CREATE INDEX IF NOT EXISTS users_role_idx        ON users(role_id);
CREATE INDEX IF NOT EXISTS users_is_verified_idx ON users(is_verified);

-- (опционально) делаем телефон уникальным
-- CREATE UNIQUE INDEX IF NOT EXISTS users_phone_uq
--   ON users (phone)
--   WHERE phone IS NOT NULL AND phone <> '';

-- (опционально) case-insensitive email (если хочешь строго в нижнем регистре)
-- CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_uq
--   ON users (lower(email));

-- ==== LEADS ===========================================================
CREATE TABLE IF NOT EXISTS leads (
                                     id          SERIAL PRIMARY KEY,
                                     title       VARCHAR(255) NOT NULL,
                                     description TEXT,
                                     owner_id    INT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
                                     status      VARCHAR(100),
                                     created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS leads_owner_idx  ON leads(owner_id);
CREATE INDEX IF NOT EXISTS leads_status_idx ON leads(status);

-- ==== DEALS ===========================================================
CREATE TABLE IF NOT EXISTS deals (
                                     id         SERIAL PRIMARY KEY,
                                     lead_id    INT REFERENCES leads(id) ON DELETE SET NULL,
                                     owner_id   INT REFERENCES users(id) ON DELETE SET NULL,
                                     amount     VARCHAR(20)  NOT NULL,
                                     currency   VARCHAR(10)  NOT NULL,
                                     status     VARCHAR(100),
                                     created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS deals_lead_idx   ON deals(lead_id);
CREATE INDEX IF NOT EXISTS deals_owner_idx  ON deals(owner_id);
CREATE INDEX IF NOT EXISTS deals_status_idx ON deals(status);

-- ==== DOCUMENTS =======================================================
-- каскад по deal_id решает удаление документов при удалении сделки
CREATE TABLE IF NOT EXISTS documents (
                                         id        SERIAL PRIMARY KEY,
                                         deal_id   INT REFERENCES deals(id) ON DELETE CASCADE,
                                         doc_type  VARCHAR(100),
                                         file_path VARCHAR(255),
                                         status    VARCHAR(100),
                                         signed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS documents_deal_idx   ON documents(deal_id);
CREATE INDEX IF NOT EXISTS documents_status_idx ON documents(status);

-- ==== MESSAGES ========================================================
CREATE TABLE IF NOT EXISTS messages (
                                        id          SERIAL PRIMARY KEY,
                                        sender_id   INT REFERENCES users(id) ON DELETE SET NULL,
                                        receiver_id INT REFERENCES users(id) ON DELETE SET NULL,
                                        content     TEXT NOT NULL,
                                        sent_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS messages_sender_idx   ON messages(sender_id);
CREATE INDEX IF NOT EXISTS messages_receiver_idx ON messages(receiver_id);

-- ==== TASKS ===========================================================
CREATE TABLE IF NOT EXISTS tasks (
                                     id          SERIAL PRIMARY KEY,
                                     creator_id  INT REFERENCES users(id) ON DELETE SET NULL,
                                     assignee_id INT REFERENCES users(id) ON DELETE SET NULL,
                                     entity_id   INT NOT NULL,
                                     entity_type VARCHAR(100) NOT NULL,  -- 'deal' | 'lead'
                                     title       TEXT NOT NULL,
                                     description TEXT,
                                     due_date    TIMESTAMPTZ,
                                     status      VARCHAR(100)
);

CREATE INDEX IF NOT EXISTS tasks_creator_idx  ON tasks(creator_id);
CREATE INDEX IF NOT EXISTS tasks_assignee_idx ON tasks(assignee_id);
CREATE INDEX IF NOT EXISTS tasks_entity_idx   ON tasks(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS tasks_status_idx   ON tasks(status);

-- ==== SMS CONFIRMATIONS (для ДОКУМЕНТОВ) ==============================
-- хранит коды в явном виде только для операций по документам
CREATE TABLE IF NOT EXISTS sms_confirmations (
                                                 id           SERIAL PRIMARY KEY,
                                                 document_id  INT REFERENCES documents(id) ON DELETE CASCADE,
                                                 sms_code     VARCHAR(100),
                                                 sent_at      TIMESTAMPTZ,
                                                 confirmed    BOOLEAN DEFAULT FALSE,
                                                 confirmed_at TIMESTAMPTZ,
                                                 phone        VARCHAR(20)
);

CREATE INDEX IF NOT EXISTS sms_document_idx  ON sms_confirmations(document_id);
CREATE INDEX IF NOT EXISTS sms_confirmed_idx ON sms_confirmations(confirmed);

-- ==== USER VERIFICATIONS (для РЕГИСТРАЦИИ/ПОДТВЕРЖДЕНИЯ ПОЛЬЗОВАТЕЛЕЙ) ===
-- безопасно храним ТОЛЬКО bcrypt-хэш кода + TTL + attempts + поля для троттлинга resend
CREATE TABLE IF NOT EXISTS user_verifications (
                                                  id             SERIAL PRIMARY KEY,
                                                  user_id        INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                                                  code_hash      TEXT        NOT NULL,            -- bcrypt-хэш кода
                                                  sent_at        TIMESTAMPTZ NOT NULL,
                                                  expires_at     TIMESTAMPTZ NOT NULL,           -- когда код протухает
                                                  confirmed      BOOLEAN     NOT NULL DEFAULT FALSE,
                                                  attempts       INT         NOT NULL DEFAULT 0,  -- сколько раз пробовали подтвердить
                                                  confirmed_at   TIMESTAMPTZ,
                                                  last_resend_at TIMESTAMPTZ,                    -- для троттлинга resend
                                                  resend_count   INT         NOT NULL DEFAULT 0,  -- кол-во resend в текущем окне
                                                  CONSTRAINT user_verifications_attempts_chk     CHECK (attempts >= 0),
                                                  CONSTRAINT user_verifications_resend_count_chk CHECK (resend_count >= 0)
);

-- Быстрый доступ к последней записи
CREATE INDEX IF NOT EXISTS user_verif_user_sent_idx
    ON user_verifications(user_id, sent_at DESC);

CREATE INDEX IF NOT EXISTS user_verif_confirmed_idx
    ON user_verifications(confirmed);

-- ==== SEED ROLES ======================================================
INSERT INTO roles (id, name, description) VALUES
                                              (10, 'sales',       'Продажник: создает/обрабатывает лиды, начинает сделки, формирует договоры.'),
                                              (15, 'staff',       'Сотрудник: доступ к задачам и сообщениям.'),
                                              (20, 'operations',  'Операционный отдел: проверяет документы от продажника, передаёт назад для подписания.'),
                                              (30, 'audit',       'Отдел контроля: read-only доступ ко всем данным (частичные маскировки).'),
                                              (40, 'management',  'Руководство: полный доступ ко всем данным.'),
                                              (50, 'admin',       'Администратор: управление системой, ролями и настройками.')
ON CONFLICT (id) DO NOTHING;

COMMIT;
