package models

import "time"

type WazzupIntegration struct {
	ID           int       `json:"id"`
	OwnerUserID  int       `json:"owner_user_id"`
	APIKeyEnc    string    `json:"api_key_enc"`
	CRMKeyHash   string    `json:"crm_key_hash"`
	WebhookToken string    `json:"webhook_token"`
	Enabled      bool      `json:"enabled"`
	WebhooksURI  string    `json:"webhooks_uri"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type WazzupDedupEvent struct {
	ID            int64     `json:"id"`
	IntegrationID int       `json:"integration_id"`
	ExternalID    string    `json:"external_id"`
	ReceivedAt    time.Time `json:"received_at"`
}
