package models

type RegisterConfirmRequest struct {
	UserID int    `json:"user_id" binding:"required"`
	Code   string `json:"code" binding:"required"`
}

type RegisterResendRequest struct {
	UserID int `json:"user_id" binding:"required"`
}
