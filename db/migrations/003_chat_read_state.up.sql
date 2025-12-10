CREATE TABLE IF NOT EXISTS chat_read_state (
    chat_id INT NOT NULL,
    user_id INT NOT NULL,
    last_read_message_id INT,
    PRIMARY KEY (chat_id, user_id)
);
