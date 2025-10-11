package models

type User struct {
	ID           int    `json:"id"`
	CompanyName  string `json:"company_name"`
	BinIin       string `json:"bin_iin"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	RoleID       int    `json:"role_id"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
