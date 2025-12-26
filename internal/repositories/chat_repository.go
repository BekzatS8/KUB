package repositories

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lib/pq"

	"turcompany/internal/models"
)

type ChatRepository interface {
	ListUserChats(userID int) ([]*models.Chat, error)
	ListMessages(chatID int, limit, offset int) ([]*models.ChatMessage, error)
	CreateMessage(chatID, senderID int, text string, attachments []string) (*models.ChatMessage, error)
	IsMember(chatID, userID int) (bool, error)
	CreateChat(name string, isGroup bool, memberIDs []int) (*models.Chat, error)
	AddMembers(chatID int, memberIDs []int) error
	RemoveMember(chatID int, userID int) error
	DeleteChat(chatID int) error
	GetChatByID(chatID int) (*models.Chat, error)
	LastMessage(chatID int) (*models.ChatMessage, error)
	SetOnline(userID int, online bool) error
	GetOnlineStatus(userID int) (bool, time.Time, error)
	UpdateLastRead(chatID, userID, messageID int) error
	CountUnread(chatID, userID int) (int, error)
	SearchChats(userID int, query string) ([]*models.Chat, error)
	SearchMessages(chatID, userID int, query string) ([]*models.ChatMessage, error)
}

type chatRepository struct {
	DB *sql.DB
}

func NewChatRepository(db *sql.DB) ChatRepository {
	return &chatRepository{DB: db}
}

func (r *chatRepository) ListUserChats(userID int) ([]*models.Chat, error) {
	const q = `
SELECT c.id,
       c.name,
       c.is_group,
       c.created_at,
       COALESCE(array_agg(cm.user_id ORDER BY cm.user_id), '{}') AS members,
       lm.text AS last_message_text,
       lm.created_at AS last_message_at,
       us.online,
       us.last_seen,
       COALESCE(unread.unread_count, 0) AS unread_count
FROM chats c
JOIN chat_members cm ON cm.chat_id = c.id
LEFT JOIN LATERAL (
    SELECT text, created_at
    FROM messages m
    WHERE m.chat_id = c.id
    ORDER BY created_at DESC, id DESC
    LIMIT 1
) lm ON true
LEFT JOIN LATERAL (
    SELECT COALESCE(status.online, false) AS online, status.last_seen
    FROM chat_members cm2
    LEFT JOIN user_status status ON status.user_id = cm2.user_id
    WHERE cm2.chat_id = c.id AND cm2.user_id <> $1
    ORDER BY cm2.user_id
    LIMIT 1
) us ON true
LEFT JOIN LATERAL (
    SELECT COUNT(*) AS unread_count
    FROM messages m
    LEFT JOIN chat_read_state crs ON crs.chat_id = m.chat_id AND crs.user_id = $1
    WHERE m.chat_id = c.id AND m.id > COALESCE(crs.last_read_message_id, 0)
) unread ON true
WHERE c.id IN (SELECT chat_id FROM chat_members WHERE user_id = $1)
GROUP BY c.id, c.name, c.is_group, c.created_at, lm.text, lm.created_at, us.online, us.last_seen, unread.unread_count
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
		var (
			members     pq.Int64Array
			lastText    sql.NullString
			lastAt      sql.NullTime
			online      sql.NullBool
			lastSeen    sql.NullTime
			unreadCount sql.NullInt64
		)

		if err := rows.Scan(
			&chat.ID,
			&chat.Name,
			&chat.IsGroup,
			&chat.CreatedAt,
			&members,
			&lastText,
			&lastAt,
			&online,
			&lastSeen,
			&unreadCount,
		); err != nil {
			return nil, err
		}

		for _, m := range members {
			chat.Members = append(chat.Members, int(m))
		}
		if lastText.Valid {
			chat.LastMessageText = lastText.String
		}
		if lastAt.Valid {
			chat.LastMessageAt = lastAt.Time
		}
		if online.Valid {
			chat.Online = online.Bool
		}
		if lastSeen.Valid {
			chat.LastSeen = lastSeen.Time
		}
		if unreadCount.Valid {
			chat.UnreadCount = int(unreadCount.Int64)
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
			// может быть "null" или "", поэтому просто пытаемся распарсить
			if err := json.Unmarshal(attachmentsBytes, &msg.Attachments); err != nil {
				return nil, err
			}
		}

		messages = append(messages, &msg)
	}

	return messages, rows.Err()
}

func (r *chatRepository) CreateMessage(chatID, senderID int, text string, attachments []string) (*models.ChatMessage, error) {
	if attachments == nil {
		attachments = []string{}
	}
	attJSON, err := json.Marshal(attachments)
	if err != nil {
		return nil, err
	}

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
SELECT 1
FROM chat_members
WHERE chat_id = $1 AND user_id = $2
LIMIT 1
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

func (r *chatRepository) CreateChat(name string, isGroup bool, memberIDs []int) (*models.Chat, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	const insertChat = `
INSERT INTO chats (name, is_group)
VALUES ($1, $2)
RETURNING id, created_at
`
	chat := &models.Chat{IsGroup: isGroup, Name: name}
	if err := tx.QueryRow(insertChat, name, isGroup).Scan(&chat.ID, &chat.CreatedAt); err != nil {
		return nil, err
	}

	const insertMember = `
INSERT INTO chat_members (chat_id, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`
	for _, memberID := range memberIDs {
		if _, err := tx.Exec(insertMember, chat.ID, memberID); err != nil {
			return nil, err
		}
		chat.Members = append(chat.Members, memberID)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return chat, nil
}

func (r *chatRepository) AddMembers(chatID int, memberIDs []int) error {
	const q = `
INSERT INTO chat_members (chat_id, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`
	for _, memberID := range memberIDs {
		if _, err := r.DB.Exec(q, chatID, memberID); err != nil {
			return err
		}
	}
	return nil
}

func (r *chatRepository) RemoveMember(chatID int, userID int) error {
	const q = `
DELETE FROM chat_members
WHERE chat_id = $1 AND user_id = $2
`
	_, err := r.DB.Exec(q, chatID, userID)
	return err
}

func (r *chatRepository) DeleteChat(chatID int) error {
	const q = `DELETE FROM chats WHERE id = $1`
	_, err := r.DB.Exec(q, chatID)
	return err
}

func (r *chatRepository) SetOnline(userID int, online bool) error {
	const q = `
INSERT INTO user_status (user_id, online, last_seen)
VALUES ($1, $2, NOW())
ON CONFLICT (user_id) DO UPDATE
SET online = EXCLUDED.online, last_seen = NOW()
`
	_, err := r.DB.Exec(q, userID, online)
	return err
}

func (r *chatRepository) GetOnlineStatus(userID int) (bool, time.Time, error) {
	const q = `SELECT online, last_seen FROM user_status WHERE user_id = $1`
	var (
		online   bool
		lastSeen sql.NullTime
	)
	err := r.DB.QueryRow(q, userID).Scan(&online, &lastSeen)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, err
	}
	if !lastSeen.Valid {
		return online, time.Time{}, nil
	}
	return online, lastSeen.Time, nil
}

func (r *chatRepository) GetChatByID(chatID int) (*models.Chat, error) {
	const q = `
SELECT c.id, c.name, c.is_group, c.created_at,
       COALESCE(array_agg(cm.user_id ORDER BY cm.user_id), '{}') AS members,
       lm.text AS last_message_text,
       lm.created_at AS last_message_at
FROM chats c
LEFT JOIN chat_members cm ON cm.chat_id = c.id
LEFT JOIN LATERAL (
    SELECT text, created_at
    FROM messages m
    WHERE m.chat_id = c.id
    ORDER BY created_at DESC, id DESC
    LIMIT 1
) lm ON true
WHERE c.id = $1
GROUP BY c.id, c.name, c.is_group, c.created_at, lm.text, lm.created_at
`
	var (
		chat    models.Chat
		members pq.Int64Array
		last    sql.NullString
		lastAt  sql.NullTime
	)

	err := r.DB.QueryRow(q, chatID).Scan(&chat.ID, &chat.Name, &chat.IsGroup, &chat.CreatedAt, &members, &last, &lastAt)
	if err != nil {
		return nil, err
	}

	for _, m := range members {
		chat.Members = append(chat.Members, int(m))
	}
	if last.Valid {
		chat.LastMessageText = last.String
	}
	if lastAt.Valid {
		chat.LastMessageAt = lastAt.Time
	}

	return &chat, nil
}

func (r *chatRepository) LastMessage(chatID int) (*models.ChatMessage, error) {
	const q = `
SELECT id, chat_id, sender_id, text, attachments, created_at
FROM messages
WHERE chat_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1
`
	var (
		msg              models.ChatMessage
		attachmentsBytes []byte
	)
	if err := r.DB.QueryRow(q, chatID).Scan(&msg.ID, &msg.ChatID, &msg.SenderID, &msg.Text, &attachmentsBytes, &msg.CreatedAt); err != nil {
		return nil, err
	}
	if len(attachmentsBytes) > 0 {
		if err := json.Unmarshal(attachmentsBytes, &msg.Attachments); err != nil {
			return nil, err
		}
	}
	return &msg, nil
}

func (r *chatRepository) UpdateLastRead(chatID, userID, messageID int) error {
	const q = `
INSERT INTO chat_read_state (chat_id, user_id, last_read_message_id)
VALUES ($1, $2, $3)
ON CONFLICT (chat_id, user_id) DO UPDATE
SET last_read_message_id = EXCLUDED.last_read_message_id
`
	_, err := r.DB.Exec(q, chatID, userID, messageID)
	return err
}

func (r *chatRepository) CountUnread(chatID, userID int) (int, error) {
	const q = `
SELECT COUNT(*)
FROM messages m
LEFT JOIN chat_read_state crs ON crs.chat_id = m.chat_id AND crs.user_id = $2
WHERE m.chat_id = $1 AND m.id > COALESCE(crs.last_read_message_id, 0)
`
	var count int
	if err := r.DB.QueryRow(q, chatID, userID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *chatRepository) SearchChats(userID int, query string) ([]*models.Chat, error) {
	prefix := query + "%"
	pattern := "%" + query + "%"

	const q = `
SELECT c.id,
       c.name,
       c.is_group,
       c.created_at,
       COALESCE(array_agg(cm.user_id ORDER BY cm.user_id), '{}') AS members,
       lm.text AS last_message_text,
       lm.created_at AS last_message_at,
       us.online,
       us.last_seen,
       COALESCE(unread.unread_count, 0) AS unread_count,
       CASE WHEN c.name ILIKE $2 THEN 0
            WHEN COALESCE(lm.text, '') ILIKE $2 THEN 1
            ELSE 2 END AS prefix_rank,
       CASE WHEN c.name ILIKE $3 THEN 0
            WHEN COALESCE(lm.text, '') ILIKE $3 THEN 1
            ELSE 2 END AS contains_rank
FROM chats c
JOIN chat_members cm ON cm.chat_id = c.id
LEFT JOIN LATERAL (
    SELECT text, created_at
    FROM messages m
    WHERE m.chat_id = c.id
    ORDER BY created_at DESC, id DESC
    LIMIT 1
) lm ON true
LEFT JOIN LATERAL (
    SELECT COALESCE(status.online, false) AS online, status.last_seen
    FROM chat_members cm2
    LEFT JOIN user_status status ON status.user_id = cm2.user_id
    WHERE cm2.chat_id = c.id AND cm2.user_id <> $1
    ORDER BY cm2.user_id
    LIMIT 1
) us ON true
LEFT JOIN LATERAL (
    SELECT COUNT(*) AS unread_count
    FROM messages m
    LEFT JOIN chat_read_state crs ON crs.chat_id = m.chat_id AND crs.user_id = $1
    WHERE m.chat_id = c.id AND m.id > COALESCE(crs.last_read_message_id, 0)
) unread ON true
WHERE c.id IN (SELECT chat_id FROM chat_members WHERE user_id = $1)
  AND (c.name ILIKE $3 OR COALESCE(lm.text, '') ILIKE $3 OR c.name ILIKE $2 OR COALESCE(lm.text, '') ILIKE $2)
GROUP BY c.id, c.name, c.is_group, c.created_at, lm.text, lm.created_at, us.online, us.last_seen, unread.unread_count, prefix_rank, contains_rank
ORDER BY c.is_group ASC, prefix_rank, contains_rank, c.id
`
	rows, err := r.DB.Query(q, userID, prefix, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []*models.Chat
	for rows.Next() {
		chat := &models.Chat{}
		var (
			members      pq.Int64Array
			lastText     sql.NullString
			lastAt       sql.NullTime
			online       sql.NullBool
			lastSeen     sql.NullTime
			unreadCount  sql.NullInt64
			prefixRank   int // ✅ вместо &_
			containsRank int // ✅ вместо &_2
		)

		if err := rows.Scan(
			&chat.ID,
			&chat.Name,
			&chat.IsGroup,
			&chat.CreatedAt,
			&members,
			&lastText,
			&lastAt,
			&online,
			&lastSeen,
			&unreadCount,
			&prefixRank,
			&containsRank,
		); err != nil {
			return nil, err
		}

		_ = prefixRank
		_ = containsRank

		for _, m := range members {
			chat.Members = append(chat.Members, int(m))
		}
		if lastText.Valid {
			chat.LastMessageText = lastText.String
		}
		if lastAt.Valid {
			chat.LastMessageAt = lastAt.Time
		}
		if online.Valid {
			chat.Online = online.Bool
		}
		if lastSeen.Valid {
			chat.LastSeen = lastSeen.Time
		}
		if unreadCount.Valid {
			chat.UnreadCount = int(unreadCount.Int64)
		}

		chats = append(chats, chat)
	}

	return chats, rows.Err()
}

func (r *chatRepository) SearchMessages(chatID, userID int, query string) ([]*models.ChatMessage, error) {
	prefix := query + "%"
	pattern := "%" + query + "%"

	const q = `
SELECT m.id, m.chat_id, m.sender_id, m.text, m.attachments, m.created_at,
       CASE WHEN m.text ILIKE $3 THEN 0 ELSE 1 END AS prefix_rank
FROM messages m
WHERE m.chat_id = $1
  AND EXISTS (SELECT 1 FROM chat_members cm WHERE cm.chat_id = m.chat_id AND cm.user_id = $2)
  AND m.text ILIKE $4
ORDER BY prefix_rank, m.created_at DESC, m.id DESC
`
	rows, err := r.DB.Query(q, chatID, userID, prefix, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*models.ChatMessage
	for rows.Next() {
		var (
			msg              models.ChatMessage
			attachmentsBytes []byte
			prefixRank       int // ✅ вместо &_
		)

		if err := rows.Scan(&msg.ID, &msg.ChatID, &msg.SenderID, &msg.Text, &attachmentsBytes, &msg.CreatedAt, &prefixRank); err != nil {
			return nil, err
		}
		_ = prefixRank

		if len(attachmentsBytes) > 0 {
			if err := json.Unmarshal(attachmentsBytes, &msg.Attachments); err != nil {
				return nil, err
			}
		}
		messages = append(messages, &msg)
	}

	return messages, rows.Err()
}
