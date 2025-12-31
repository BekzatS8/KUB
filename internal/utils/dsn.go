package utils

import "net/url"

// MaskDSN returns a safe DSN string without user credentials.
func MaskDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "invalid-dsn"
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "invalid-dsn"
	}

	parsed.User = nil
	return parsed.String()
}
