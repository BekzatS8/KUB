package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientProfilesMigrationContainsBackfill(t *testing.T) {
	path := filepath.Join("021_client_profiles_split.up.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	s := string(b)
	checks := []string{
		"CREATE TABLE IF NOT EXISTS client_individual_profiles",
		"CREATE TABLE IF NOT EXISTS client_legal_profiles",
		"INSERT INTO client_individual_profiles",
		"INSERT INTO client_legal_profiles",
	}
	for _, check := range checks {
		if !strings.Contains(s, check) {
			t.Fatalf("migration missing fragment %q", check)
		}
	}
}
