package handlers

import (
	"fmt"
	"strings"
	"time"
)

const supportedDateFormats = "YYYY-MM-DD, DD.MM.YYYY, YYYY-MM-DDTHH:MM:SSZ"

type dateFieldError struct {
	field string
}

func (e *dateFieldError) Error() string {
	return fmt.Sprintf("invalid %s format. Allowed formats: %s", e.field, supportedDateFormats)
}

func parseFlexibleDate(field, value string, required bool) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return nil, &dateFieldError{field: field}
		}
		return nil, nil
	}
	layouts := []string{time.DateOnly, "02.01.2006", time.RFC3339}
	for _, layout := range layouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			u := t.UTC()
			return &u, nil
		}
	}
	return nil, &dateFieldError{field: field}
}
