package models

import (
	"encoding/json"
	"time"
)

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

	Phone               string     `json:"phone"`
	Email               string     `json:"email"`
	RegistrationAddress string     `json:"registration_address"`
	ActualAddress       string     `json:"actual_address"`
	Country             string     `json:"country,omitempty"`
	TripPurpose         string     `json:"trip_purpose,omitempty"`
	BirthDate           *time.Time `json:"birth_date,omitempty"`
	BirthPlace          string     `json:"birth_place,omitempty"`
	Citizenship         string     `json:"citizenship,omitempty"`
	Sex                 string     `json:"sex,omitempty"`
	MaritalStatus       string     `json:"marital_status,omitempty"`
	PassportIssueDate   *time.Time `json:"passport_issue_date,omitempty"`
	PassportExpireDate  *time.Time `json:"passport_expire_date,omitempty"`

	PreviousLastName        string          `json:"previous_last_name,omitempty"`
	SpouseName              string          `json:"spouse_name,omitempty"`
	SpouseContacts          string          `json:"spouse_contacts,omitempty"`
	HasChildren             *bool           `json:"has_children,omitempty"`
	ChildrenList            json.RawMessage `json:"children_list,omitempty"`
	Education               string          `json:"education,omitempty"`
	Job                     string          `json:"job,omitempty"`
	TripsLast5Years         string          `json:"trips_last5_years,omitempty"`
	RelativesInDestination  string          `json:"relatives_in_destination,omitempty"`
	TrustedPerson           string          `json:"trusted_person,omitempty"`
	Height                  *int16          `json:"height,omitempty"`
	Weight                  *int16          `json:"weight,omitempty"`
	DriverLicenseCategories json.RawMessage `json:"driver_license_categories,omitempty"`
	TherapistName           string          `json:"therapist_name,omitempty"`
	ClinicName              string          `json:"clinic_name,omitempty"`
	DiseasesLast3Years      string          `json:"diseases_last3_years,omitempty"`
	AdditionalInfo          string          `json:"additional_info,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}
