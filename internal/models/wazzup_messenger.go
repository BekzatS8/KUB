package models

import (
	"encoding/json"
	"time"
)

type WazzupChannel struct {
	ID                int64           `json:"id"`
	IntegrationID     int             `json:"integration_id"`
	ExternalChannelID string          `json:"channel_id"`
	Transport         string          `json:"transport"`
	Name              string          `json:"name"`
	Username          string          `json:"username,omitempty"`
	Phone             string          `json:"phone,omitempty"`
	Status            string          `json:"status"`
	Provider          string          `json:"provider"`
	RawPayload        json.RawMessage `json:"raw_payload,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type WazzupStatus struct {
	Configured      bool       `json:"configured"`
	Enabled         bool       `json:"enabled"`
	IframeAvailable bool       `json:"iframe_available"`
	ChannelsCount   int        `json:"channels_count"`
	LastWebhookAt   *time.Time `json:"last_webhook_at,omitempty"`
	Provider        string     `json:"provider"`
}

type WazzupDialog struct {
	ID                int        `json:"id"`
	Provider          string     `json:"provider"`
	Transport         string     `json:"transport"`
	ExternalChatID    string     `json:"external_chat_id"`
	ExternalChannelID string     `json:"external_channel_id"`
	DisplayName       string     `json:"display_name"`
	Username          string     `json:"username,omitempty"`
	Phone             string     `json:"phone,omitempty"`
	LastMessageText   string     `json:"last_message_text"`
	LastMessageAt     *time.Time `json:"last_message_at,omitempty"`
	UnreadCount       int        `json:"unread_count"`
	ClientID          *int       `json:"client_id,omitempty"`
	LeadID            *int       `json:"lead_id,omitempty"`
	BranchID          *int       `json:"branch_id,omitempty"`
}

type WazzupDialogMessage struct {
	ID                int             `json:"id"`
	ChatID            int             `json:"chat_id"`
	SenderID          *int            `json:"sender_id,omitempty"`
	Text              string          `json:"text"`
	Direction         string          `json:"direction"`
	Status            string          `json:"status"`
	Transport         string          `json:"transport"`
	ExternalMessageID string          `json:"external_message_id,omitempty"`
	ExternalChannelID string          `json:"external_channel_id,omitempty"`
	RawPayload        json.RawMessage `json:"raw_payload,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}
