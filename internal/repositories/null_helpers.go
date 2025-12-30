package repositories

import "database/sql"

func stringFromNull(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func nullStringFromEmpty(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
