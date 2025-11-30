package models

import "time"

type Chat struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	IsGroup   bool      `json:"is_group"`
	Members   []int     `json:"members"`
	CreatedAt time.Time `json:"created_at"`
}

type ChatMessage struct {
	ID          int       `json:"id"`
	ChatID      int       `json:"chat_id"`
	SenderID    int       `json:"sender_id"`
	Text        string    `json:"text"`
	Attachments []string  `json:"attachments"`
	CreatedAt   time.Time `json:"created_at"`
}
