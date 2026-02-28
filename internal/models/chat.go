package models

import "time"

const (
	ChatMemberRoleOwner  = "owner"
	ChatMemberRoleAdmin  = "admin"
	ChatMemberRoleMember = "member"
)

type Chat struct {
	ID              int          `json:"id"`
	CreatorID       int          `json:"creator_id"`
	Name            string       `json:"name"`
	IsGroup         bool         `json:"is_group"`
	Members         []int        `json:"members"`
	MemberStatuses  []UserStatus `json:"member_statuses,omitempty"`
	LastMessageText string       `json:"last_message_text"`
	LastMessageAt   time.Time    `json:"last_message_at"`
	Online          bool         `json:"online"`
	LastSeen        time.Time    `json:"last_seen"`
	UnreadCount     int          `json:"unread_count"`
	CreatedAt       time.Time    `json:"created_at"`
}

type ChatInfoResponse struct {
	Chat         ChatInfoMeta          `json:"chat"`
	Participants []ChatInfoParticipant `json:"participants"`
}

type ChatInfoMeta struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	IsGroup   bool      `json:"is_group"`
	CreatorID int       `json:"creator_id"`
	CreatedAt time.Time `json:"created_at"`
	ClientID  *string   `json:"client_id,omitempty"`
	DealID    *string   `json:"deal_id,omitempty"`
	LeadID    *string   `json:"lead_id,omitempty"`
}

type ChatInfoParticipant struct {
	UserID            int        `json:"user_id"`
	Role              string     `json:"role"`
	JoinedAt          time.Time  `json:"joined_at"`
	Email             string     `json:"email"`
	DisplayName       string     `json:"display_name"`
	AvatarURL         *string    `json:"avatar_url,omitempty"`
	Online            bool       `json:"online"`
	LastSeen          *time.Time `json:"last_seen,omitempty"`
	LastReadMessageID *int       `json:"last_read_message_id,omitempty"`
	ReadAt            *time.Time `json:"read_at,omitempty"`
}

type ChatReadEvent struct {
	Type              string    `json:"type"`
	ChatID            int       `json:"chat_id"`
	UserID            int       `json:"user_id"`
	LastReadMessageID int       `json:"last_read_message_id"`
	ReadAt            time.Time `json:"read_at"`
}

// UserStatus describes the current online state of a user inside a chat context.
type UserStatus struct {
	UserID   int       `json:"user_id"`
	IsOnline bool      `json:"is_online"`
	LastSeen time.Time `json:"last_seen"`
}

type ChatMessage struct {
	ID           int        `json:"id"`
	ChatID       int        `json:"chat_id"`
	SenderID     int        `json:"sender_id"`
	Text         string     `json:"text"`
	Attachments  []string   `json:"attachments"`
	CreatedAt    time.Time  `json:"created_at"`
	EditedAt     *time.Time `json:"edited_at,omitempty"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
	DeletedBy    *int       `json:"deleted_by,omitempty"`
	IsDeleted    bool       `json:"is_deleted"`
	DeleteReason *string    `json:"delete_reason,omitempty"`
}

type Attachment struct {
	ID            string    `json:"id"`
	ChatID        int       `json:"chat_id"`
	MessageID     *int      `json:"message_id,omitempty"`
	UploaderID    int       `json:"uploader_id"`
	FileName      string    `json:"file_name"`
	MimeType      string    `json:"mime_type"`
	SizeBytes     int64     `json:"size_bytes"`
	StorageDriver string    `json:"storage_driver"`
	StorageKey    string    `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
}

type AttachmentResponse struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	FileName  string `json:"file_name"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

type PinResponse struct {
	MessageID int       `json:"message_id"`
	PinnedBy  int       `json:"pinned_by"`
	PinnedAt  time.Time `json:"pinned_at"`
}

type FavoriteResponse struct {
	MessageID int       `json:"message_id"`
	CreatedAt time.Time `json:"created_at"`
}
