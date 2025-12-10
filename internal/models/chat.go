package models

import "time"

type Chat struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	IsGroup         bool      `json:"is_group"`
	Members         []int     `json:"members"`
	LastMessageText string    `json:"last_message_text"`
	LastMessageAt   time.Time `json:"last_message_at"`
	Online          bool      `json:"online"`
	LastSeen        time.Time `json:"last_seen"`
	UnreadCount     int       `json:"unread_count"`
	CreatedAt       time.Time `json:"created_at"`
}

type ChatMessage struct {
	ID          int       `json:"id"`
	ChatID      int       `json:"chat_id"`
	SenderID    int       `json:"sender_id"`
	Text        string    `json:"text"`
	Attachments []string  `json:"attachments"`
	CreatedAt   time.Time `json:"created_at"`
}
