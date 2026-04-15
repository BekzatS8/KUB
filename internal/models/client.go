package models

import (
	"encoding/json"
	"time"
)

// Client представляет клиента (физлицо или компанию).
type Client struct {
	ID           int    `json:"id"`
	OwnerID      int    `json:"owner_id"`
	ClientType   string `json:"client_type" db:"client_type"`
	DisplayName  string `json:"display_name"`
	PrimaryPhone string `json:"primary_phone"`
	PrimaryEmail string `json:"primary_email"`
	Name         string `json:"name"`         // Отображаемое имя (для юзера, компания/ФИО)
	BinIin       string `json:"bin_iin"`      // БИН/ИИН (как было)
	Address      string `json:"address"`      // Общий адрес (можно использовать как фактический)
	ContactInfo  string `json:"contact_info"` // Доп. инфа (телеграм и т.п.)

	// Поля анкеты (физ. лицо)
	LastName   string `json:"last_name"`   // Фамилия
	FirstName  string `json:"first_name"`  // Имя
	MiddleName string `json:"middle_name"` // Отчество

	IIN            string `json:"iin"`             // ИИН
	IDNumber       string `json:"id_number"`       // № удостоверения
	PassportSeries string `json:"passport_series"` // Серия паспорта
	PassportNumber string `json:"passport_number"` // № паспорта

	Phone                   string     `json:"phone"`
	Email                   string     `json:"email"`
	RegistrationAddress     string     `json:"registration_address"`
	ActualAddress           string     `json:"actual_address"`
	Country                 string     `json:"country,omitempty"`
	TripPurpose             string     `json:"trip_purpose,omitempty"`
	BirthDate               *time.Time `json:"birth_date,omitempty"`
	BirthPlace              string     `json:"birth_place,omitempty"`
	Citizenship             string     `json:"citizenship,omitempty"`
	Sex                     string     `json:"sex,omitempty"`
	MaritalStatus           string     `json:"marital_status,omitempty"`
	PassportIssueDate       *time.Time `json:"passport_issue_date,omitempty"`
	PassportExpireDate      *time.Time `json:"passport_expire_date,omitempty"`
	DriverLicenseIssueDate  *time.Time `json:"driver_license_issue_date,omitempty"`
	DriverLicenseExpireDate *time.Time `json:"driver_license_expire_date,omitempty"`

	PreviousLastName            string          `json:"previous_last_name,omitempty"`
	SpouseName                  string          `json:"spouse_name,omitempty"`
	SpouseContacts              string          `json:"spouse_contacts,omitempty"`
	HasChildren                 *bool           `json:"has_children,omitempty"`
	ChildrenList                json.RawMessage `json:"children_list,omitempty"`
	Education                   string          `json:"education,omitempty"`
	EducationLevel              string          `json:"education_level,omitempty"`
	Job                         string          `json:"job,omitempty"`
	TripsLast5Years             string          `json:"trips_last5_years,omitempty"`
	RelativesInDestination      string          `json:"relatives_in_destination,omitempty"`
	TrustedPerson               string          `json:"trusted_person,omitempty"`
	Specialty                   string          `json:"specialty,omitempty"`
	TrustedPersonPhone          string          `json:"trusted_person_phone,omitempty"`
	DriverLicenseNumber         string          `json:"driver_license_number,omitempty"`
	EducationInstitutionName    string          `json:"education_institution_name,omitempty"`
	EducationInstitutionAddress string          `json:"education_institution_address,omitempty"`
	Position                    string          `json:"position,omitempty"`
	VisasReceived               string          `json:"visas_received,omitempty"`
	VisaRefusals                string          `json:"visa_refusals,omitempty"`
	Height                      *int16          `json:"height,omitempty"`
	Weight                      *int16          `json:"weight,omitempty"`
	DriverLicenseCategories     json.RawMessage `json:"driver_license_categories,omitempty"`
	TherapistName               string          `json:"therapist_name,omitempty"`
	ClinicName                  string          `json:"clinic_name,omitempty"`
	DiseasesLast3Years          string          `json:"diseases_last3_years,omitempty"`
	AdditionalInfo              string          `json:"additional_info,omitempty"`

	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	IsArchived    bool       `json:"is_archived"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	ArchivedBy    *int       `json:"archived_by,omitempty"`
	ArchiveReason string     `json:"archive_reason,omitempty"`

	IndividualProfile *ClientIndividualProfile `json:"individual_profile,omitempty"`
	LegalProfile      *ClientLegalProfile      `json:"legal_profile,omitempty"`
}

// TypedClientRef — явная типизированная ссылка на клиента.
type TypedClientRef struct {
	ClientID   int    `json:"client_id"`
	ClientType string `json:"client_type"`
}

func (c *Client) TypedRef() TypedClientRef {
	if c == nil {
		return TypedClientRef{}
	}
	return TypedClientRef{
		ClientID:   c.ID,
		ClientType: c.ClientType,
	}
}

type ClientIndividualProfile struct {
	ClientID                    int             `json:"client_id"`
	LastName                    string          `json:"last_name"`
	FirstName                   string          `json:"first_name"`
	MiddleName                  string          `json:"middle_name"`
	IIN                         string          `json:"iin"`
	IDNumber                    string          `json:"id_number"`
	PassportSeries              string          `json:"passport_series"`
	PassportNumber              string          `json:"passport_number"`
	RegistrationAddress         string          `json:"registration_address"`
	ActualAddress               string          `json:"actual_address"`
	Country                     string          `json:"country,omitempty"`
	TripPurpose                 string          `json:"trip_purpose,omitempty"`
	BirthDate                   *time.Time      `json:"birth_date,omitempty"`
	BirthPlace                  string          `json:"birth_place,omitempty"`
	Citizenship                 string          `json:"citizenship,omitempty"`
	Sex                         string          `json:"sex,omitempty"`
	MaritalStatus               string          `json:"marital_status,omitempty"`
	PassportIssueDate           *time.Time      `json:"passport_issue_date,omitempty"`
	PassportExpireDate          *time.Time      `json:"passport_expire_date,omitempty"`
	DriverLicenseIssueDate      *time.Time      `json:"driver_license_issue_date,omitempty"`
	DriverLicenseExpireDate     *time.Time      `json:"driver_license_expire_date,omitempty"`
	PreviousLastName            string          `json:"previous_last_name,omitempty"`
	SpouseName                  string          `json:"spouse_name,omitempty"`
	SpouseContacts              string          `json:"spouse_contacts,omitempty"`
	HasChildren                 *bool           `json:"has_children,omitempty"`
	ChildrenList                json.RawMessage `json:"children_list,omitempty"`
	Education                   string          `json:"education,omitempty"`
	EducationLevel              string          `json:"education_level,omitempty"`
	Job                         string          `json:"job,omitempty"`
	TripsLast5Years             string          `json:"trips_last5_years,omitempty"`
	RelativesInDestination      string          `json:"relatives_in_destination,omitempty"`
	TrustedPerson               string          `json:"trusted_person,omitempty"`
	Specialty                   string          `json:"specialty,omitempty"`
	TrustedPersonPhone          string          `json:"trusted_person_phone,omitempty"`
	DriverLicenseNumber         string          `json:"driver_license_number,omitempty"`
	EducationInstitutionName    string          `json:"education_institution_name,omitempty"`
	EducationInstitutionAddress string          `json:"education_institution_address,omitempty"`
	Position                    string          `json:"position,omitempty"`
	VisasReceived               string          `json:"visas_received,omitempty"`
	VisaRefusals                string          `json:"visa_refusals,omitempty"`
	Height                      *int16          `json:"height,omitempty"`
	Weight                      *int16          `json:"weight,omitempty"`
	DriverLicenseCategories     json.RawMessage `json:"driver_license_categories,omitempty"`
	TherapistName               string          `json:"therapist_name,omitempty"`
	ClinicName                  string          `json:"clinic_name,omitempty"`
	DiseasesLast3Years          string          `json:"diseases_last3_years,omitempty"`
	AdditionalInfo              string          `json:"additional_info,omitempty"`
}

type ClientLegalProfile struct {
	ClientID              int    `json:"client_id"`
	CompanyName           string `json:"company_name"`
	BIN                   string `json:"bin"`
	LegalForm             string `json:"legal_form,omitempty"`
	DirectorFullName      string `json:"director_full_name,omitempty"`
	ContactPersonName     string `json:"contact_person_name,omitempty"`
	ContactPersonPosition string `json:"contact_person_position,omitempty"`
	ContactPersonPhone    string `json:"contact_person_phone,omitempty"`
	ContactPersonEmail    string `json:"contact_person_email,omitempty"`
	LegalAddress          string `json:"legal_address,omitempty"`
	ActualAddress         string `json:"actual_address,omitempty"`
	BankName              string `json:"bank_name,omitempty"`
	IBAN                  string `json:"iban,omitempty"`
	BIK                   string `json:"bik,omitempty"`
	KBE                   string `json:"kbe,omitempty"`
	TaxRegime             string `json:"tax_regime,omitempty"`
	Website               string `json:"website,omitempty"`
	Industry              string `json:"industry,omitempty"`
	CompanySize           string `json:"company_size,omitempty"`
	AdditionalInfo        string `json:"additional_info,omitempty"`
}

const (
	ClientTypeIndividual = "individual"
	ClientTypeLegal      = "legal"
)
