package repositories

import (
	"context"
	"database/sql"
	"turcompany/internal/models"
)

// MessageRepository defines the interface for database operations on messages.
type MessageRepository interface {
	Store(ctx context.Context, msg *models.Message) error
	FindConversationHistory(ctx context.Context, userID1, userID2 int64) ([]models.Message, error)
	FindConversations(ctx context.Context, userID int64) ([]models.Conversation, error)
}

type messageRepository struct {
	db *sql.DB
}

// NewMessageRepository creates a new instance of MessageRepository.
func NewMessageRepository(db *sql.DB) MessageRepository {
	return &messageRepository{db: db}
}

func (r *messageRepository) Store(ctx context.Context, msg *models.Message) error {
	query := `INSERT INTO messages (sender_id, receiver_id, content, sent_at) VALUES ($1, $2, $3, $4) RETURNING id, sent_at`
	return r.db.QueryRowContext(ctx, query, msg.SenderID, msg.ReceiverID, msg.Content, msg.SentAt).Scan(&msg.ID, &msg.SentAt)
}

func (r *messageRepository) FindConversationHistory(ctx context.Context, userID1, userID2 int64) ([]models.Message, error) {
	query := `
		SELECT id, sender_id, receiver_id, content, sent_at, read_at FROM messages
		WHERE (sender_id = $1 AND receiver_id = $2) OR (sender_id = $2 AND receiver_id = $1)
		ORDER BY sent_at ASC`

	rows, err := r.db.QueryContext(ctx, query, userID1, userID2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var msg models.Message
		if err := rows.Scan(&msg.ID, &msg.SenderID, &msg.ReceiverID, &msg.Content, &msg.SentAt, &msg.ReadAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// FindConversations is a complex query. It finds all unique chat partners for a user,
// gets the last message for each, and counts unread messages.
func (r *messageRepository) FindConversations(ctx context.Context, userID int64) ([]models.Conversation, error) {
	// This query is complex and might need optimization for very large datasets (e.g., using window functions).
	// For now, this is a straightforward approach.
	query := `
		WITH message_partners AS (
			SELECT
				CASE
					WHEN sender_id = $1 THEN receiver_id
					ELSE sender_id
				END as partner_id,
				MAX(sent_at) as max_sent_at
			FROM messages
			WHERE sender_id = $1 OR receiver_id = $1
			GROUP BY partner_id
		)
		SELECT
			p.partner_id,
			u.company_name,
			u.email,
			m.id,
			m.sender_id,
			m.receiver_id,
			m.content,
			m.sent_at,
			m.read_at,
			(SELECT COUNT(*) FROM messages um WHERE um.sender_id = p.partner_id AND um.receiver_id = $1 AND um.read_at IS NULL) as unread_count
		FROM message_partners p
		JOIN messages m ON (
			(m.sender_id = p.partner_id AND m.receiver_id = $1) OR
			(m.sender_id = $1 AND m.receiver_id = p.partner_id)
		) AND m.sent_at = p.max_sent_at
		JOIN users u ON u.id = p.partner_id
		ORDER BY m.sent_at DESC`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []models.Conversation
	for rows.Next() {
		var conv models.Conversation
		if err := rows.Scan(
			&conv.Partner.ID, &conv.Partner.CompanyName, &conv.Partner.Email,
			&conv.LastMessage.ID, &conv.LastMessage.SenderID, &conv.LastMessage.ReceiverID,
			&conv.LastMessage.Content, &conv.LastMessage.SentAt, &conv.LastMessage.ReadAt,
			&conv.UnreadCount,
		); err != nil {
			return nil, err
		}
		conversations = append(conversations, conv)
	}
	return conversations, nil
}
