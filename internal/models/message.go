package models

import "time"

// Message represents a single message between two users.
type Message struct {
	ID         int64      `json:"id"`
	SenderID   int64      `json:"sender_id"`
	ReceiverID int64      `json:"receiver_id"`
	Content    string     `json:"content"`
	SentAt     time.Time  `json:"sent_at"`
	ReadAt     *time.Time `json:"read_at,omitempty"` // Time the message was read, null if unread
}

// Conversation represents a summary of a chat between the current user and a partner.
type Conversation struct {
	Partner     User    `json:"partner"`
	LastMessage Message `json:"last_message"`
	UnreadCount int     `json:"unread_count"`
}
