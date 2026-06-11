package models

type Role struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code,omitempty"`
	Description string `json:"description"`
}
