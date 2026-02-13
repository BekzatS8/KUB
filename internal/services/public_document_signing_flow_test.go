package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"turcompany/internal/repositories"
)

func TestPublicDocumentSigningFlow_DB(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}
	db := openTestDB(t, dsn)
	defer db.Close()
	applyAllMigrations(t, db)

	userID := mustCreateUser(t, db, fmt.Sprintf("public_sign_user_%d@example.com", time.Now().UnixNano()), 40)
	docID := mustCreateApprovedDocument(t, db, "contract")

	docRepo := repositories.NewDocumentRepository(db)
	docSvc := NewDocumentService(docRepo, nil, nil, nil, "", t.TempDir(), nil, nil, nil)
	linkRepo := repositories.NewPublicDocumentLinkRepository(db)
	service := NewPublicDocumentSigningService(linkRepo, docSvc, docRepo, PublicDocumentSigningConfig{
		BaseURL:     "http://localhost:4000",
		TokenPepper: "test-pepper",
		TTLMinutes:  30,
	}, nil)

	signURL, expiresAt, err := service.GenerateSignLink(context.Background(), docID, userID, 40, 30)
	if err != nil {
		t.Fatalf("generate sign link: %v", err)
	}
	if expiresAt.IsZero() {
		t.Fatalf("expected expiresAt")
	}
	rawToken := tokenFromURL(t, signURL)

	publicDoc, publicExpiresAt, err := service.GetPublicDocument(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("get public document: %v", err)
	}
	if publicDoc.ID != docID {
		t.Fatalf("doc id mismatch: got %d want %d", publicDoc.ID, docID)
	}
	if publicExpiresAt.IsZero() {
		t.Fatalf("expected public expiresAt")
	}

	signedAt, eventID, signedDocID, err := service.SignPublicDocument(context.Background(), rawToken, PublicDocumentSignPayload{
		SignerName:  "Client One",
		SignerEmail: "client@example.com",
		SignerPhone: "+77010000000",
		Signature:   "test-signature",
	}, "127.0.0.1", "GoTest")
	if err != nil {
		t.Fatalf("sign public document: %v", err)
	}
	if signedAt.IsZero() || eventID == "" || signedDocID != docID {
		t.Fatalf("unexpected sign output: signedAt=%v event=%q doc=%d", signedAt, eventID, signedDocID)
	}

	status, signMethod, signMetadata := mustDocumentSignFields(t, db, docID)
	if status != "signed" {
		t.Fatalf("status = %s, want signed", status)
	}
	if signMethod != "public_link" {
		t.Fatalf("sign_method = %s, want public_link", signMethod)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(signMetadata), &meta); err != nil {
		t.Fatalf("unmarshal sign_metadata: %v", err)
	}
	if meta["signed_by"] != "client" {
		t.Fatalf("signed_by = %v, want client", meta["signed_by"])
	}
	if strings.TrimSpace(fmt.Sprint(meta["event_id"])) == "" {
		t.Fatalf("expected event_id in sign_metadata")
	}

	if used := mustLinkUsed(t, db, docID); !used {
		t.Fatalf("expected link used_at not null")
	}
	if count := mustSignaturesCount(t, db, docID); count < 1 {
		t.Fatalf("expected signatures count > 0")
	}
}

func TestPublicDocumentSigningFlow_Errors_DB(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}
	db := openTestDB(t, dsn)
	defer db.Close()
	applyAllMigrations(t, db)

	userID := mustCreateUser(t, db, fmt.Sprintf("public_sign_user2_%d@example.com", time.Now().UnixNano()), 40)
	docID := mustCreateApprovedDocument(t, db, "contract")

	docRepo := repositories.NewDocumentRepository(db)
	docSvc := NewDocumentService(docRepo, nil, nil, nil, "", t.TempDir(), nil, nil, nil)
	linkRepo := repositories.NewPublicDocumentLinkRepository(db)
	service := NewPublicDocumentSigningService(linkRepo, docSvc, docRepo, PublicDocumentSigningConfig{
		BaseURL:     "http://localhost:4000",
		TokenPepper: "test-pepper",
		TTLMinutes:  1,
	}, nil)

	signURL, _, err := service.GenerateSignLink(context.Background(), docID, userID, 40, 1)
	if err != nil {
		t.Fatalf("generate sign link: %v", err)
	}
	rawToken := tokenFromURL(t, signURL)

	_, _, _, err = service.SignPublicDocument(context.Background(), rawToken, PublicDocumentSignPayload{
		SignerName: "Client Two",
		Signature:  "sig",
	}, "127.0.0.1", "GoTest")
	if err != nil {
		t.Fatalf("first sign: %v", err)
	}
	_, _, _, err = service.SignPublicDocument(context.Background(), rawToken, PublicDocumentSignPayload{
		SignerName: "Client Two",
		Signature:  "sig",
	}, "127.0.0.1", "GoTest")
	if !errors.Is(err, repositories.ErrPublicLinkUsed) {
		t.Fatalf("expected ErrPublicLinkUsed, got %v", err)
	}

	otherDocID := mustCreateApprovedDocument(t, db, "invoice")
	otherURL, _, err := service.GenerateSignLink(context.Background(), otherDocID, userID, 40, 1)
	if err != nil {
		t.Fatalf("generate second sign link: %v", err)
	}
	otherToken := tokenFromURL(t, otherURL)
	if _, err := db.Exec(`UPDATE public_document_links SET expires_at = NOW() - INTERVAL '1 minute' WHERE document_id = $1`, otherDocID); err != nil {
		t.Fatalf("expire link: %v", err)
	}
	_, _, err = service.GetPublicDocument(context.Background(), otherToken)
	if !errors.Is(err, repositories.ErrPublicLinkExpired) {
		t.Fatalf("expected ErrPublicLinkExpired, got %v", err)
	}

	_, _, err = service.GetPublicDocument(context.Background(), "missing-token")
	if !errors.Is(err, repositories.ErrPublicLinkNotFound) {
		t.Fatalf("expected ErrPublicLinkNotFound, got %v", err)
	}
}

func openTestDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	return db
}

func applyAllMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	migrationsDir := filepath.Join(wd, "..", "..", "db", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	for _, name := range files {
		content, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}

func mustCreateUser(t *testing.T, db *sql.DB, email string, roleID int) int {
	t.Helper()
	var id int
	err := db.QueryRow(`INSERT INTO users (email, password_hash, role_id) VALUES ($1,$2,$3) RETURNING id`, email, "hash", roleID).Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

func mustCreateApprovedDocument(t *testing.T, db *sql.DB, docType string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		INSERT INTO documents (deal_id, doc_type, file_path, file_path_pdf, status)
		VALUES (NULL, $1, $2, $3, 'approved')
		RETURNING id
	`, docType, "pdf/test.pdf", "pdf/test.pdf").Scan(&id)
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	return id
}

func tokenFromURL(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 {
		t.Fatalf("unexpected public sign url path: %s", u.Path)
	}
	return parts[len(parts)-1]
}

func mustDocumentSignFields(t *testing.T, db *sql.DB, docID int64) (string, string, string) {
	t.Helper()
	var status, signMethod string
	var signMetadata sql.NullString
	err := db.QueryRow(`SELECT status, COALESCE(sign_method,''), sign_metadata FROM documents WHERE id = $1`, docID).Scan(&status, &signMethod, &signMetadata)
	if err != nil {
		t.Fatalf("load doc sign fields: %v", err)
	}
	return status, signMethod, signMetadata.String
}

func mustLinkUsed(t *testing.T, db *sql.DB, docID int64) bool {
	t.Helper()
	var usedAt sql.NullTime
	err := db.QueryRow(`SELECT used_at FROM public_document_links WHERE document_id = $1 ORDER BY id DESC LIMIT 1`, docID).Scan(&usedAt)
	if err != nil {
		t.Fatalf("load link used_at: %v", err)
	}
	return usedAt.Valid && usedAt.Time.Before(time.Now().Add(1*time.Minute))
}

func mustSignaturesCount(t *testing.T, db *sql.DB, docID int64) int {
	t.Helper()
	var count int
	err := db.QueryRow(`SELECT COUNT(1) FROM public_document_signatures WHERE document_id = $1`, docID).Scan(&count)
	if err != nil {
		t.Fatalf("count signatures: %v", err)
	}
	return count
}
