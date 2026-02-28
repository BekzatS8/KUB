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
	CreateChat(name string, isGroup bool, creatorID int, memberIDs []int) (*models.Chat, error)
	AddMembers(chatID int, memberIDs []int) error
	RemoveMember(chatID int, userID int) error
	DeleteChat(chatID int) error
	GetMemberRole(chatID, userID int) (string, error)
	GetChatInfo(chatID, userID int) (*models.ChatInfoResponse, error)
	GetChatByID(chatID int) (*models.Chat, error)
	LastMessage(chatID int) (*models.ChatMessage, error)
	SetOnline(userID int, online bool) error
	GetOnlineStatus(userID int) (bool, time.Time, error)
	UpdateLastRead(chatID, userID, messageID int) error
	MarkChatRead(chatID, userID int, messageID *int) (int, time.Time, error)
	CountUnread(chatID, userID int) (int, error)
	SearchChats(userID int, query string) ([]*models.Chat, error)
	SearchMessagesFTS(chatID, userID int, query string, limit, offset int) ([]*models.ChatMessage, error)
	SearchMessagesILIKE(chatID, userID int, query string, limit, offset int) ([]*models.ChatMessage, error)
	CreateAttachment(chatID, uploaderID int, fileName, mime string, size int64, storageKey string) (*models.Attachment, error)
	AttachToMessage(attachmentIDs []string, messageID, chatID, uploaderID int) error
	GetAttachmentsByMessageIDs(messageIDs []int) (map[int][]models.AttachmentResponse, error)
	GetAttachmentForDownload(id string) (*models.Attachment, error)
	EditMessage(chatID, messageID, editorUserID int, newText string) (*models.ChatMessage, error)
	DeleteMessage(chatID, messageID, userID int) (*models.ChatMessage, error)
	PinMessage(chatID, messageID, userID int) (*models.PinResponse, error)
	UnpinMessage(chatID, messageID, userID int) error
	FavoriteMessage(chatID, messageID, userID int) (*models.FavoriteResponse, error)
	UnfavoriteMessage(chatID, messageID, userID int) error
	ListPins(chatID, userID, limit, offset int) ([]*models.PinResponse, error)
	ListFavorites(chatID, userID, limit, offset int) ([]*models.FavoriteResponse, error)
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
SELECT id, chat_id, sender_id, text, attachments, created_at, edited_at, deleted_at, deleted_by, delete_reason
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
	return scanChatMessages(rows)
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

func (r *chatRepository) CreateChat(name string, isGroup bool, creatorID int, memberIDs []int) (*models.Chat, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	const insertChat = `
INSERT INTO chats (name, creator_id, is_group)
VALUES ($1, $2, $3)
RETURNING id, creator_id, created_at
`
	chat := &models.Chat{IsGroup: isGroup, Name: name}
	if err := tx.QueryRow(insertChat, name, creatorID, isGroup).Scan(&chat.ID, &chat.CreatorID, &chat.CreatedAt); err != nil {
		return nil, err
	}

	const insertMember = `
INSERT INTO chat_members (chat_id, user_id, role)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING
`
	for _, memberID := range memberIDs {
		role := models.ChatMemberRoleMember
		if memberID == creatorID {
			role = models.ChatMemberRoleOwner
		}
		if _, err := tx.Exec(insertMember, chat.ID, memberID, role); err != nil {
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
INSERT INTO chat_members (chat_id, user_id, role)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING
`
	for _, memberID := range memberIDs {
		if _, err := r.DB.Exec(q, chatID, memberID, models.ChatMemberRoleMember); err != nil {
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

func (r *chatRepository) GetMemberRole(chatID, userID int) (string, error) {
	const q = `
SELECT role
FROM chat_members
WHERE chat_id = $1 AND user_id = $2
`
	var role string
	if err := r.DB.QueryRow(q, chatID, userID).Scan(&role); err != nil {
		return "", err
	}
	return role, nil
}

func (r *chatRepository) GetChatInfo(chatID, userID int) (*models.ChatInfoResponse, error) {
	ok, err := r.IsMember(chatID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		if _, err := r.GetChatByID(chatID); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}

	const chatQ = `
SELECT id, name, is_group, creator_id, created_at, client_id::text, deal_id::text, lead_id::text
FROM chats
WHERE id = $1
`
	info := &models.ChatInfoResponse{}
	var clientID, dealID, leadID sql.NullString
	if err := r.DB.QueryRow(chatQ, chatID).Scan(
		&info.Chat.ID,
		&info.Chat.Name,
		&info.Chat.IsGroup,
		&info.Chat.CreatorID,
		&info.Chat.CreatedAt,
		&clientID,
		&dealID,
		&leadID,
	); err != nil {
		return nil, err
	}
	if clientID.Valid {
		info.Chat.ClientID = &clientID.String
	}
	if dealID.Valid {
		info.Chat.DealID = &dealID.String
	}
	if leadID.Valid {
		info.Chat.LeadID = &leadID.String
	}

	const membersQ = `
SELECT cm.user_id,
       cm.role,
       cm.joined_at,
       u.email,
       COALESCE(NULLIF(u.company_name, ''), u.email) AS display_name,
       NULL::text AS avatar_url,
       COALESCE(us.online, false) AS online,
       us.last_seen,
       crs.last_read_message_id,
       crs.read_at
FROM chat_members cm
JOIN users u ON u.id = cm.user_id
LEFT JOIN user_status us ON us.user_id = cm.user_id
LEFT JOIN chat_read_state crs ON crs.chat_id = cm.chat_id AND crs.user_id = cm.user_id
WHERE cm.chat_id = $1
ORDER BY CASE cm.role
            WHEN 'owner' THEN 0
            WHEN 'admin' THEN 1
            ELSE 2
         END,
         cm.joined_at ASC,
         cm.user_id ASC
`
	rows, err := r.DB.Query(membersQ, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var p models.ChatInfoParticipant
		var avatar sql.NullString
		var lastSeen sql.NullTime
		var lastReadID sql.NullInt64
		var readAt sql.NullTime
		if err := rows.Scan(&p.UserID, &p.Role, &p.JoinedAt, &p.Email, &p.DisplayName, &avatar, &p.Online, &lastSeen, &lastReadID, &readAt); err != nil {
			return nil, err
		}
		if avatar.Valid {
			p.AvatarURL = &avatar.String
		}
		if lastSeen.Valid {
			t := lastSeen.Time
			p.LastSeen = &t
		}
		if lastReadID.Valid {
			v := int(lastReadID.Int64)
			p.LastReadMessageID = &v
		}
		if readAt.Valid {
			t := readAt.Time
			p.ReadAt = &t
		}
		info.Participants = append(info.Participants, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return info, nil
}

func (r *chatRepository) CreateAttachment(chatID, uploaderID int, fileName, mime string, size int64, storageKey string) (*models.Attachment, error) {
	const q = `
INSERT INTO attachments (chat_id, uploader_id, file_name, mime_type, size_bytes, storage_driver, storage_key)
VALUES ($1, $2, $3, $4, $5, 'local', $6)
RETURNING id::text, chat_id, message_id, uploader_id, file_name, mime_type, size_bytes, storage_driver, storage_key, created_at
`
	att := &models.Attachment{}
	var msgID sql.NullInt64
	if err := r.DB.QueryRow(q, chatID, uploaderID, fileName, mime, size, storageKey).Scan(
		&att.ID, &att.ChatID, &msgID, &att.UploaderID, &att.FileName, &att.MimeType, &att.SizeBytes, &att.StorageDriver, &att.StorageKey, &att.CreatedAt,
	); err != nil {
		return nil, err
	}
	if msgID.Valid {
		v := int(msgID.Int64)
		att.MessageID = &v
	}
	return att, nil
}

func (r *chatRepository) AttachToMessage(attachmentIDs []string, messageID, chatID, uploaderID int) error {
	const q = `
UPDATE attachments
SET message_id = $1
WHERE id = $2::uuid AND chat_id = $3 AND uploader_id = $4 AND message_id IS NULL
`
	for _, id := range attachmentIDs {
		res, err := r.DB.Exec(q, messageID, id, chatID, uploaderID)
		if err != nil {
			return err
		}
		aff, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if aff == 0 {
			return sql.ErrNoRows
		}
	}
	return nil
}

func (r *chatRepository) GetAttachmentsByMessageIDs(messageIDs []int) (map[int][]models.AttachmentResponse, error) {
	res := make(map[int][]models.AttachmentResponse)
	if len(messageIDs) == 0 {
		return res, nil
	}
	const q = `
SELECT id::text, message_id, file_name, mime_type, size_bytes
FROM attachments
WHERE message_id = ANY($1)
ORDER BY created_at ASC
`
	rows, err := r.DB.Query(q, pq.Array(messageIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id, fileName, mime string
			msgID              int
			size               int64
		)
		if err := rows.Scan(&id, &msgID, &fileName, &mime, &size); err != nil {
			return nil, err
		}
		res[msgID] = append(res[msgID], models.AttachmentResponse{ID: id, URL: "/attachments/" + id + "/download", FileName: fileName, MimeType: mime, SizeBytes: size})
	}
	return res, rows.Err()
}

func (r *chatRepository) GetAttachmentForDownload(id string) (*models.Attachment, error) {
	const q = `
SELECT a.id::text, a.chat_id, a.message_id, a.uploader_id, a.file_name, a.mime_type, a.size_bytes, a.storage_driver, a.storage_key, a.created_at
FROM attachments a
LEFT JOIN messages m ON m.id = a.message_id
WHERE a.id = $1::uuid
  AND (a.message_id IS NULL OR m.deleted_at IS NULL)
`
	att := &models.Attachment{}
	var msgID sql.NullInt64
	if err := r.DB.QueryRow(q, id).Scan(&att.ID, &att.ChatID, &msgID, &att.UploaderID, &att.FileName, &att.MimeType, &att.SizeBytes, &att.StorageDriver, &att.StorageKey, &att.CreatedAt); err != nil {
		return nil, err
	}
	if msgID.Valid {
		v := int(msgID.Int64)
		att.MessageID = &v
	}
	return att, nil
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
SELECT id, chat_id, sender_id, text, attachments, created_at, edited_at, deleted_at, deleted_by, delete_reason
FROM messages
WHERE chat_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1
`
	rows, err := r.DB.Query(q, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	msgs, err := scanChatMessages(rows)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, sql.ErrNoRows
	}
	return msgs[0], nil
}

func (r *chatRepository) UpdateLastRead(chatID, userID, messageID int) error {
	const q = `
INSERT INTO chat_read_state (chat_id, user_id, last_read_message_id, read_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (chat_id, user_id) DO UPDATE
SET last_read_message_id = EXCLUDED.last_read_message_id,
    read_at = NOW()
`
	_, err := r.DB.Exec(q, chatID, userID, messageID)
	return err
}

func (r *chatRepository) MarkChatRead(chatID, userID int, messageID *int) (int, time.Time, error) {
	if messageID != nil {
		const checkQ = `SELECT 1 FROM messages WHERE id = $1 AND chat_id = $2`
		var ok int
		if err := r.DB.QueryRow(checkQ, *messageID, chatID).Scan(&ok); err != nil {
			return 0, time.Time{}, err
		}
		if err := r.UpdateLastRead(chatID, userID, *messageID); err != nil {
			return 0, time.Time{}, err
		}
		return *messageID, time.Now(), nil
	}

	lastMsg, err := r.LastMessage(chatID)
	if err != nil {
		if err == sql.ErrNoRows {
			if err := r.UpdateLastRead(chatID, userID, 0); err != nil {
				return 0, time.Time{}, err
			}
			return 0, time.Now(), nil
		}
		return 0, time.Time{}, err
	}
	if err := r.UpdateLastRead(chatID, userID, lastMsg.ID); err != nil {
		return 0, time.Time{}, err
	}
	return lastMsg.ID, time.Now(), nil
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

func (r *chatRepository) SearchMessagesFTS(chatID, userID int, query string, limit, offset int) ([]*models.ChatMessage, error) {
	const q = `
SELECT m.id, m.chat_id, m.sender_id, m.text, m.attachments, m.created_at, m.edited_at, m.deleted_at, m.deleted_by, m.delete_reason
FROM messages m
WHERE m.chat_id = $1
  AND EXISTS (SELECT 1 FROM chat_members cm WHERE cm.chat_id = m.chat_id AND cm.user_id = $2)
  AND m.deleted_at IS NULL
  AND m.search_tsv @@ plainto_tsquery('simple', $3)
ORDER BY ts_rank(m.search_tsv, plainto_tsquery('simple', $3)) DESC, m.created_at DESC, m.id DESC
LIMIT $4 OFFSET $5
`
	rows, err := r.DB.Query(q, chatID, userID, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChatMessages(rows)
}

func (r *chatRepository) SearchMessagesILIKE(chatID, userID int, query string, limit, offset int) ([]*models.ChatMessage, error) {
	pattern := "%" + query + "%"
	const q = `
SELECT m.id, m.chat_id, m.sender_id, m.text, m.attachments, m.created_at, m.edited_at, m.deleted_at, m.deleted_by, m.delete_reason
FROM messages m
WHERE m.chat_id = $1
  AND EXISTS (SELECT 1 FROM chat_members cm WHERE cm.chat_id = m.chat_id AND cm.user_id = $2)
  AND m.deleted_at IS NULL
  AND m.text ILIKE $3
ORDER BY m.created_at DESC, m.id DESC
LIMIT $4 OFFSET $5
`
	rows, err := r.DB.Query(q, chatID, userID, pattern, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChatMessages(rows)
}

func scanChatMessages(rows *sql.Rows) ([]*models.ChatMessage, error) {
	var messages []*models.ChatMessage
	for rows.Next() {
		var (
			msg              models.ChatMessage
			attachmentsBytes []byte
			editedAt         sql.NullTime
			deletedAt        sql.NullTime
			deletedBy        sql.NullInt64
			deleteReason     sql.NullString
		)
		if err := rows.Scan(&msg.ID, &msg.ChatID, &msg.SenderID, &msg.Text, &attachmentsBytes, &msg.CreatedAt, &editedAt, &deletedAt, &deletedBy, &deleteReason); err != nil {
			return nil, err
		}
		if len(attachmentsBytes) > 0 {
			if err := json.Unmarshal(attachmentsBytes, &msg.Attachments); err != nil {
				return nil, err
			}
		}
		if editedAt.Valid {
			t := editedAt.Time
			msg.EditedAt = &t
		}
		if deletedAt.Valid {
			t := deletedAt.Time
			msg.DeletedAt = &t
			msg.IsDeleted = true
			msg.Text = "[deleted]"
			msg.Attachments = []string{}
		}
		if deletedBy.Valid {
			v := int(deletedBy.Int64)
			msg.DeletedBy = &v
		}
		if deleteReason.Valid {
			msg.DeleteReason = &deleteReason.String
		}
		messages = append(messages, &msg)
	}
	return messages, rows.Err()
}

func (r *chatRepository) EditMessage(chatID, messageID, editorUserID int, newText string) (*models.ChatMessage, error) {
	const q = `
UPDATE messages m
SET text = $1, edited_at = NOW()
WHERE m.id = $2
  AND m.chat_id = $3
  AND m.deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM chat_members cm WHERE cm.chat_id = $3 AND cm.user_id = $4)
  AND m.sender_id = $4
RETURNING m.id, m.chat_id, m.sender_id, m.text, m.attachments, m.created_at, m.edited_at, m.deleted_at, m.deleted_by, m.delete_reason
`
	rows, err := r.DB.Query(q, newText, messageID, chatID, editorUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	msgs, err := scanChatMessages(rows)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, sql.ErrNoRows
	}
	return msgs[0], nil
}

func (r *chatRepository) DeleteMessage(chatID, messageID, userID int) (*models.ChatMessage, error) {
	role, _ := r.GetMemberRole(chatID, userID)
	const q = `
UPDATE messages m
SET deleted_at = NOW(), deleted_by = $1, delete_reason = 'deleted', text = ''
WHERE m.id = $2
  AND m.chat_id = $3
  AND m.deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM chat_members cm WHERE cm.chat_id = $3 AND cm.user_id = $1)
  AND (m.sender_id = $1 OR $4 IN ('owner','admin'))
RETURNING m.id, m.chat_id, m.sender_id, m.text, m.attachments, m.created_at, m.edited_at, m.deleted_at, m.deleted_by, m.delete_reason
`
	rows, err := r.DB.Query(q, userID, messageID, chatID, role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	msgs, err := scanChatMessages(rows)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, sql.ErrNoRows
	}
	return msgs[0], nil
}

func (r *chatRepository) PinMessage(chatID, messageID, userID int) (*models.PinResponse, error) {
	role, err := r.GetMemberRole(chatID, userID)
	if err != nil {
		return nil, err
	}
	if role != models.ChatMemberRoleOwner && role != models.ChatMemberRoleAdmin {
		return nil, sql.ErrNoRows
	}
	const q = `
INSERT INTO chat_pins (chat_id, message_id, pinned_by)
SELECT $1, m.id, $3 FROM messages m WHERE m.id = $2 AND m.chat_id = $1
ON CONFLICT (chat_id, message_id) DO UPDATE SET pinned_by = EXCLUDED.pinned_by, pinned_at = NOW()
RETURNING message_id, pinned_by, pinned_at
`
	var p models.PinResponse
	if err := r.DB.QueryRow(q, chatID, messageID, userID).Scan(&p.MessageID, &p.PinnedBy, &p.PinnedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *chatRepository) UnpinMessage(chatID, messageID, userID int) error {
	role, err := r.GetMemberRole(chatID, userID)
	if err != nil {
		return err
	}
	if role != models.ChatMemberRoleOwner && role != models.ChatMemberRoleAdmin {
		return sql.ErrNoRows
	}
	_, err = r.DB.Exec(`DELETE FROM chat_pins WHERE chat_id = $1 AND message_id = $2`, chatID, messageID)
	return err
}

func (r *chatRepository) FavoriteMessage(chatID, messageID, userID int) (*models.FavoriteResponse, error) {
	const q = `
INSERT INTO message_favorites (user_id, message_id)
SELECT $1, m.id FROM messages m
WHERE m.id = $2 AND m.chat_id = $3
  AND EXISTS (SELECT 1 FROM chat_members cm WHERE cm.chat_id = $3 AND cm.user_id = $1)
ON CONFLICT (user_id, message_id) DO UPDATE SET created_at = NOW()
RETURNING message_id, created_at
`
	var f models.FavoriteResponse
	if err := r.DB.QueryRow(q, userID, messageID, chatID).Scan(&f.MessageID, &f.CreatedAt); err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *chatRepository) UnfavoriteMessage(chatID, messageID, userID int) error {
	_, err := r.DB.Exec(`DELETE FROM message_favorites WHERE user_id = $1 AND message_id = $2
AND EXISTS (SELECT 1 FROM messages m WHERE m.id = $2 AND m.chat_id = $3)
AND EXISTS (SELECT 1 FROM chat_members cm WHERE cm.chat_id = $3 AND cm.user_id = $1)`, userID, messageID, chatID)
	return err
}

func (r *chatRepository) ListPins(chatID, userID, limit, offset int) ([]*models.PinResponse, error) {
	if ok, err := r.IsMember(chatID, userID); err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return []*models.PinResponse{}, nil
	}
	rows, err := r.DB.Query(`SELECT message_id, pinned_by, pinned_at FROM chat_pins WHERE chat_id = $1 ORDER BY pinned_at DESC LIMIT $2 OFFSET $3`, chatID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.PinResponse
	for rows.Next() {
		var p models.PinResponse
		if err := rows.Scan(&p.MessageID, &p.PinnedBy, &p.PinnedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *chatRepository) ListFavorites(chatID, userID, limit, offset int) ([]*models.FavoriteResponse, error) {
	rows, err := r.DB.Query(`
SELECT mf.message_id, mf.created_at
FROM message_favorites mf
JOIN messages m ON m.id = mf.message_id
WHERE mf.user_id = $1 AND m.chat_id = $2
  AND EXISTS (SELECT 1 FROM chat_members cm WHERE cm.chat_id = $2 AND cm.user_id = $1)
ORDER BY mf.created_at DESC
LIMIT $3 OFFSET $4
`, userID, chatID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.FavoriteResponse
	for rows.Next() {
		var f models.FavoriteResponse
		if err := rows.Scan(&f.MessageID, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}
