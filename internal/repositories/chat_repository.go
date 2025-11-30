package repositories

import (
	"database/sql"
	"encoding/json"

	"github.com/lib/pq"

	"turcompany/internal/models"
)

type ChatRepository interface {
	ListUserChats(userID int) ([]*models.Chat, error)
	ListMessages(chatID int, limit, offset int) ([]*models.ChatMessage, error)
	CreateMessage(chatID, senderID int, text string, attachments []string) (*models.ChatMessage, error)
	IsMember(chatID, userID int) (bool, error)
}

type chatRepository struct {
	DB *sql.DB
}

func NewChatRepository(db *sql.DB) ChatRepository {
	return &chatRepository{DB: db}
}

func (r *chatRepository) ListUserChats(userID int) ([]*models.Chat, error) {
	const q = `
                SELECT c.id, c.name, c.is_group, c.created_at,
                       COALESCE(array_agg(cm.user_id ORDER BY cm.user_id), '{}') AS members
                FROM chats c
                JOIN chat_members cm ON cm.chat_id = c.id
                WHERE c.id IN (SELECT chat_id FROM chat_members WHERE user_id = $1)
                GROUP BY c.id, c.name, c.is_group, c.created_at
                ORDER BY c.id
        `
	rows, err := r.DB.Query(q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []*models.Chat
	for rows.Next() {
		chat := &models.Chat{}
		var members pq.Int64Array
		if err := rows.Scan(&chat.ID, &chat.Name, &chat.IsGroup, &chat.CreatedAt, &members); err != nil {
			return nil, err
		}
		for _, m := range members {
			chat.Members = append(chat.Members, int(m))
		}
		chats = append(chats, chat)
	}
	return chats, rows.Err()
}

func (r *chatRepository) ListMessages(chatID int, limit, offset int) ([]*models.ChatMessage, error) {
	const q = `
                SELECT id, chat_id, sender_id, text, attachments, created_at
                FROM messages
                WHERE chat_id = $1
                ORDER BY created_at ASC, id ASC
                LIMIT $2 OFFSET $3
        `
	rows, err := r.DB.Query(q, chatID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*models.ChatMessage
	for rows.Next() {
		var (
			msg              models.ChatMessage
			attachmentsBytes []byte
		)
		if err := rows.Scan(&msg.ID, &msg.ChatID, &msg.SenderID, &msg.Text, &attachmentsBytes, &msg.CreatedAt); err != nil {
			return nil, err
		}
		if len(attachmentsBytes) > 0 {
			_ = json.Unmarshal(attachmentsBytes, &msg.Attachments)
		}
		messages = append(messages, &msg)
	}
	return messages, rows.Err()
}

func (r *chatRepository) CreateMessage(chatID, senderID int, text string, attachments []string) (*models.ChatMessage, error) {
	attJSON, _ := json.Marshal(attachments)
	const q = `
                INSERT INTO messages (chat_id, sender_id, text, attachments)
                VALUES ($1, $2, $3, $4)
                RETURNING id, created_at
        `
	msg := &models.ChatMessage{
		ChatID:      chatID,
		SenderID:    senderID,
		Text:        text,
		Attachments: attachments,
	}
	if err := r.DB.QueryRow(q, chatID, senderID, text, attJSON).Scan(&msg.ID, &msg.CreatedAt); err != nil {
		return nil, err
	}
	return msg, nil
}

func (r *chatRepository) IsMember(chatID, userID int) (bool, error) {
	const q = `
                SELECT 1 FROM chat_members WHERE chat_id = $1 AND user_id = $2 LIMIT 1
        `
	var dummy int
	err := r.DB.QueryRow(q, chatID, userID).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
