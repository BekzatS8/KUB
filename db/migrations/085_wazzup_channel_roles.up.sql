-- Доступ сотрудников к чатам номера Wazzup, настроенный вручную (как окно
-- «Выбор ролей» в кабинете Wazzup). У дочернего White Label аккаунта кабинета
-- нет, роли на его номерах выдаёт CRM. Пока roles_configured = FALSE, роли
-- номера выдаются автоматически по ролям CRM.
ALTER TABLE wazzup_channels ADD COLUMN IF NOT EXISTS roles_configured BOOLEAN NOT NULL DEFAULT FALSE;

-- role: seller — «Менеджер» (видит своих клиентов), manager — «Руководитель»
-- (видит все чаты), auditor — «Контроль качества» (видит все, писать не может).
CREATE TABLE IF NOT EXISTS wazzup_channel_user_roles (
    channel_id            BIGINT      NOT NULL REFERENCES wazzup_channels(id) ON DELETE CASCADE,
    user_id               INT         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role                  TEXT        NOT NULL CHECK (role IN ('seller', 'manager', 'auditor')),
    allow_get_new_clients BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (channel_id, user_id)
);
