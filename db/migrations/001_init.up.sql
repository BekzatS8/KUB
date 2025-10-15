-- ==== ROLES ====
CREATE TABLE IF NOT EXISTS roles (
                                     id          SERIAL PRIMARY KEY,
                                     name        VARCHAR(255) NOT NULL,
                                     description TEXT
);

-- ==== USERS ====  (+ refresh_token, refresh_expires_at)
CREATE TABLE IF NOT EXISTS users (
                                     id                SERIAL PRIMARY KEY,
                                     company_name      VARCHAR(255),
                                     bin_iin           VARCHAR(255),
                                     email             VARCHAR(255) NOT NULL UNIQUE,
                                     password_hash     VARCHAR(255) NOT NULL,
                                     role_id           INT REFERENCES roles(id),

    -- refresh storage
                                     refresh_token     TEXT,
                                     refresh_expires_at TIMESTAMPTZ,
                                     refresh_revoked   BOOLEAN NOT NULL DEFAULT FALSE
);

-- 2) Апгрейд уже существующей таблицы (если поля вдруг отсутствуют)
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS refresh_token      TEXT,
    ADD COLUMN IF NOT EXISTS refresh_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS refresh_revoked    BOOLEAN NOT NULL DEFAULT FALSE;

-- 3) Уникальность только среди НЕ-NULL refresh_token (чтобы разные NULL не конфликтовали)
CREATE UNIQUE INDEX IF NOT EXISTS users_refresh_token_uq
    ON users (refresh_token)
    WHERE refresh_token IS NOT NULL;

-- 4) Полезный индекс по роли
CREATE INDEX IF NOT EXISTS users_role_idx ON users(role_id);


-- ==== LEADS ====
CREATE TABLE IF NOT EXISTS leads (
                                     id          SERIAL PRIMARY KEY,
                                     title       VARCHAR(255) NOT NULL,
                                     description TEXT,
                                     owner_id    INT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
                                     status      VARCHAR(100),
                                     created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS leads_owner_idx ON leads(owner_id);
CREATE INDEX IF NOT EXISTS leads_status_idx ON leads(status);


-- ==== DEALS ====
CREATE TABLE IF NOT EXISTS deals (
                                     id         SERIAL PRIMARY KEY,
                                     lead_id    INT REFERENCES leads(id) ON DELETE SET NULL,
                                     owner_id   INT REFERENCES users(id) ON DELETE SET NULL,
                                     amount     VARCHAR(20)  NOT NULL,
                                     currency   VARCHAR(10)  NOT NULL,
                                     status     VARCHAR(100),
                                     created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS deals_lead_idx    ON deals(lead_id);
CREATE INDEX IF NOT EXISTS deals_owner_idx   ON deals(owner_id);
CREATE INDEX IF NOT EXISTS deals_status_idx  ON deals(status);


-- ==== DOCUMENTS ==== (ВАЖНО: каскад по deal_id → решает твою ошибку при удалении сделок)
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


-- ==== MESSAGES ====
CREATE TABLE IF NOT EXISTS messages (
                                        id          SERIAL PRIMARY KEY,
                                        sender_id   INT REFERENCES users(id) ON DELETE SET NULL,
                                        receiver_id INT REFERENCES users(id) ON DELETE SET NULL,
                                        content     TEXT NOT NULL,
                                        sent_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS messages_sender_idx   ON messages(sender_id);
CREATE INDEX IF NOT EXISTS messages_receiver_idx ON messages(receiver_id);


-- ==== TASKS ====
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
CREATE INDEX IF NOT EXISTS tasks_creator_idx    ON tasks(creator_id);
CREATE INDEX IF NOT EXISTS tasks_assignee_idx   ON tasks(assignee_id);
CREATE INDEX IF NOT EXISTS tasks_entity_idx     ON tasks(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS tasks_status_idx     ON tasks(status);


-- ==== SMS CONFIRMATIONS ==== (каскад по document_id логичен)
CREATE TABLE IF NOT EXISTS sms_confirmations (
                                                 id           SERIAL PRIMARY KEY,
                                                 document_id  INT REFERENCES documents(id) ON DELETE CASCADE,
                                                 sms_code     VARCHAR(100),
                                                 sent_at      TIMESTAMPTZ,
                                                 confirmed    BOOLEAN DEFAULT FALSE,
                                                 confirmed_at TIMESTAMPTZ,
                                                 phone        VARCHAR(20)
);
CREATE INDEX IF NOT EXISTS sms_document_idx   ON sms_confirmations(document_id);
CREATE INDEX IF NOT EXISTS sms_confirmed_idx  ON sms_confirmations(confirmed);


-- ==== SEED ROLES ====
INSERT INTO roles (id, name, description) VALUES
                                              (10, 'sales',      'Продажник: создает/обрабатывает лиды, начинает сделки, формирует договоры.'),
                                              (20, 'operations', 'Операционный отдел: проверяет документы от продажника, передаёт назад для подписания.'),
                                              (30, 'audit',      'Отдел контроля: может просматривать данные всех пользователей (кроме сведений о руководстве).'),
                                              (40, 'management', 'Руководство: полный доступ ко всем данным.'),
                                              (50, 'admin',      'Администратор: управляет системой, может назначать роли, смотреть логи, настраивать интеграции.')
ON CONFLICT (id) DO NOTHING;
