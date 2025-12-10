package models

import "time"

// Client представляет клиента (физлицо или компанию).
type Client struct {
	ID          int    `json:"id"`
	OwnerID     int    `json:"owner_id"`
	Name        string `json:"name"`         // Отображаемое имя (для юзера, компания/ФИО)
	BinIin      string `json:"bin_iin"`      // БИН/ИИН (как было)
	Address     string `json:"address"`      // Общий адрес (можно использовать как фактический)
	ContactInfo string `json:"contact_info"` // Доп. инфа (телеграм и т.п.)

	// Поля анкеты (физ. лицо)
	LastName   string `json:"last_name"`   // Фамилия
	FirstName  string `json:"first_name"`  // Имя
	MiddleName string `json:"middle_name"` // Отчество

	IIN            string `json:"iin"`             // ИИН
	IDNumber       string `json:"id_number"`       // № удостоверения
	PassportSeries string `json:"passport_series"` // Серия паспорта
	PassportNumber string `json:"passport_number"` // № паспорта

	Phone               string `json:"phone"`
	Email               string `json:"email"`
	RegistrationAddress string `json:"registration_address"`
	ActualAddress       string `json:"actual_address"`

	CreatedAt time.Time `json:"created_at"`
}
