package models

import "time"

const (
	DriveKindFolder = "folder"
	DriveKindFile   = "file"
)

// DriveNode — папка или файл в хранилище. Содержимое файла лежит в объектном
// хранилище под StorageKey, в базе только метаданные.
type DriveNode struct {
	ID            int64     `json:"id"`
	ParentID      *int64    `json:"parent_id"`
	Kind          string    `json:"kind"`
	Name          string    `json:"name"`
	StorageKey    string    `json:"-"`
	SizeBytes     int64     `json:"size_bytes"`
	MimeType      string    `json:"mime_type"`
	CreatedBy     *int      `json:"created_by,omitempty"`
	CreatedByName string    `json:"created_by_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// SharesCount — сколько пользователей имеют активный доступ именно к этому
	// узлу. Заполняется только для администратора.
	SharesCount int `json:"shares_count,omitempty"`
	// ChildrenCount — число элементов внутри папки.
	ChildrenCount int `json:"children_count,omitempty"`
	// ShareExpiresAt — до какого момента действует доступ, по которому
	// пользователь видит узел (для списка «Доступные мне»). nil — бессрочно.
	ShareExpiresAt *time.Time `json:"share_expires_at,omitempty"`
	// Preview — каким способом файл можно показать без скачивания:
	// image | pdf | video | audio | text | office. Пусто — предпросмотра нет.
	// Считается на бэкенде: офисные форматы доступны, только если на сервере
	// включён LibreOffice.
	Preview string `json:"preview,omitempty"`
}

func (n DriveNode) IsFolder() bool { return n.Kind == DriveKindFolder }

// DriveBreadcrumb — звено пути от корня до текущей папки.
type DriveBreadcrumb struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// DriveShare — выданный пользователю доступ к файлу или папке.
type DriveShare struct {
	ID            int64      `json:"id"`
	NodeID        int64      `json:"node_id"`
	UserID        int        `json:"user_id"`
	UserName      string     `json:"user_name"`
	UserEmail     string     `json:"user_email,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at"`
	Expired       bool       `json:"expired"`
	CreatedBy     *int       `json:"created_by,omitempty"`
	CreatedByName string     `json:"created_by_name,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// DriveUser — сотрудник в списке выбора при выдаче доступа.
type DriveUser struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	RoleID int    `json:"role_id"`
}
