-- Создание ролей
CREATE TABLE IF NOT EXISTS roles (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT
);

-- Создание пользователей
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    company_name VARCHAR(255),
    bin_iin VARCHAR(255),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role_id INT REFERENCES roles(id)
);

-- Создание лидов
CREATE TABLE IF NOT EXISTS leads (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    owner_id INT NOT NULL REFERENCES users(id),
    status VARCHAR(100),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Создание сделок
CREATE TABLE IF NOT EXISTS deals (
    id SERIAL PRIMARY KEY,
    lead_id INT REFERENCES leads(id),
    owner_id INT REFERENCES users(id), -- << ИСПРАВЛЕНО: Добавлен владелец сделки
    amount VARCHAR(20) NOT NULL,
    currency VARCHAR(10) NOT NULL,
    status VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW() -- << ИСПРАВЛЕНО: Тип данных
);

-- Создание сообщений
CREATE TABLE IF NOT EXISTS messages (
    id SERIAL PRIMARY KEY,
    sender_id INT REFERENCES users(id),
    receiver_id INT REFERENCES users(id),
    content TEXT NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Создание задач
CREATE TABLE IF NOT EXISTS tasks (
    id SERIAL PRIMARY KEY,
    creator_id INT REFERENCES users(id),
    assignee_id INT REFERENCES users(id),
    entity_id INT NOT NULL,
    entity_type VARCHAR(100) NOT NULL, -- 'deal' или 'lead'
    title TEXT NOT NULL,
    description TEXT,
    due_date TIMESTAMPTZ,
    status VARCHAR(100)
);

-- Создание документов
CREATE TABLE IF NOT EXISTS documents (
    id SERIAL PRIMARY KEY,
    deal_id INT REFERENCES deals(id),
    doc_type VARCHAR(100),
    file_path VARCHAR(255),
    status VARCHAR(100),
    signed_at TIMESTAMPTZ
);

-- Создание SMS-подтверждений
CREATE TABLE IF NOT EXISTS sms_confirmations (
    id SERIAL PRIMARY KEY,
    document_id INT REFERENCES documents(id),
    sms_code VARCHAR(100),
    sent_at TIMESTAMPTZ,
    confirmed BOOLEAN DEFAULT FALSE,
    confirmed_at TIMESTAMPTZ,
    phone VARCHAR(20)
);
