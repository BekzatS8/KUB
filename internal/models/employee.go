package models

import "time"

// Типы сотрудника (ТЗ «доработки CRM КУБ», п.1).
const (
	EmployeeTypePermanent = "permanent" // постоянный сотрудник
	EmployeeTypeIntern    = "intern"    // стажёр
	EmployeeTypeFired     = "fired"     // уволенный
)

// Employee — карточка сотрудника. Даты — в формате ГГГГ-ММ-ДД.
type Employee struct {
	ID                int        `json:"id"`
	FullName          string     `json:"full_name"`
	BirthDate         string     `json:"birth_date"`
	IIN               string     `json:"iin"`
	Address           string     `json:"address"`
	Phone             string     `json:"phone"`
	Email             string     `json:"email,omitempty"`
	EmergencyName     string     `json:"emergency_name"`
	EmergencyRelation string     `json:"emergency_relation"`
	EmergencyPhone    string     `json:"emergency_phone"`
	Position          string     `json:"position"`
	Salary            float64    `json:"salary"`
	HiredAt           string     `json:"hired_at"`
	FiredAt           string     `json:"fired_at,omitempty"`
	Type              string     `json:"employee_type"`
	HasPhoto          bool       `json:"has_photo"`
	PhotoKey          string     `json:"-"`
	UserID            *int       `json:"user_id,omitempty"`
	UserName          string     `json:"user_name,omitempty"`
	BranchID          *int       `json:"branch_id,omitempty"`
	BranchName        string     `json:"branch_name,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `json:"-"`
}

// EmployeeFilter — список сотрудников: поиск по ФИО/ИИН/телефону, фильтры по
// типу и должности; уволенные по умолчанию скрыты.
type EmployeeFilter struct {
	Query        string
	Type         string
	Position     string
	IncludeFired bool
	SortBy       string
	Order        string
	Limit        int
	Offset       int
}

// HRTemplate — шаблон кадрового документа (Word).
type HRTemplate struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	OriginalName string    `json:"original_name,omitempty"`
	FileKey      string    `json:"-"`
	UploadedBy   *int      `json:"uploaded_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// EmployeeDocument — кадровый документ сотрудника (строка documents, scope=hr).
// Статусы: draft (черновик) → sent_for_signature (отправлен) → signed
// (подписан); revoked — отозван.
type EmployeeDocument struct {
	ID           int64      `json:"id"`
	EmployeeID   int        `json:"employee_id"`
	TemplateID   *int       `json:"template_id,omitempty"`
	Title        string     `json:"title"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	SentAt       *time.Time `json:"sent_at,omitempty"`
	SentVia      string     `json:"sent_via,omitempty"`
	SentTo       string     `json:"sent_to,omitempty"`
	SignedAt     *time.Time `json:"signed_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	HasSignedPDF bool       `json:"has_signed_pdf"`
	FilePathPdf  string     `json:"-"`
	FilePathDocx string     `json:"-"`
	SignMetadata string     `json:"-"`
}
