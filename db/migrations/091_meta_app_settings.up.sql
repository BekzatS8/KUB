-- 091_meta_app_settings.up.sql
-- Приложение Meta компании (вход через Facebook для подключения Instagram к
-- дочернему аккаунту Wazzup). Ключи вводит администратор в CRM — без правки
-- переменных сервера. Одна строка на CRM.

CREATE TABLE IF NOT EXISTS meta_app_settings (
    id          SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    app_id      TEXT NOT NULL DEFAULT '',
    app_secret  TEXT NOT NULL DEFAULT '',
    updated_by  INT REFERENCES users(id) ON DELETE SET NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
