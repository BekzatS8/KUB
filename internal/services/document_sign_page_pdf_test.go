package services

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"turcompany/internal/models"
)

func TestBuildSigningPagePDF_UsesUTF8FontAndCreatesFile(t *testing.T) {
	svc := &DocumentService{}
	outPath := filepath.Join(t.TempDir(), "sign_page.pdf")

	doc := &models.Document{ID: 42, DocType: "contract_full"}
	signedAt := time.Now().UTC()
	session := &models.SignSession{
		SignerEmail:     "user@example.com",
		SignedAt:        &signedAt,
		SignedIP:        "127.0.0.1",
		SignedUserAgent: "go-test",
		DocHash:         "abc123",
	}

	if err := svc.buildSigningPagePDF(doc, session, outPath); err != nil {
		t.Fatalf("buildSigningPagePDF returned error: %v", err)
	}

	pdfBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected output pdf file to be created: %v", err)
	}
	if len(pdfBytes) == 0 {
		t.Fatalf("expected non-empty pdf, got size=%d", len(pdfBytes))
	}
	if bytes.Contains(pdfBytes, []byte("Helvetica")) {
		t.Fatalf("expected generated PDF to avoid Helvetica fallback")
	}
	if bytes.Contains(pdfBytes, []byte("QR: unavailable")) {
		t.Fatalf("expected generated PDF to omit QR placeholder")
	}
}

func TestBuildSigningPagePDF_HidesVerifyURLWhenNA(t *testing.T) {
	svc := &DocumentService{}
	outPath := filepath.Join(t.TempDir(), "sign_page_no_verify.pdf")

	doc := &models.Document{ID: 7, DocType: "additional_agreement", SignMetadata: `{"verify_url":"N/A"}`}
	signedAt := time.Now().UTC()
	session := &models.SignSession{
		SignerEmail:     "user@example.com",
		SignedAt:        &signedAt,
		SignedIP:        "127.0.0.1",
		SignedUserAgent: "go-test-agent",
		DocHash:         "012345678901234567890123456789012345678901234567890123456789abcd",
	}

	if err := svc.buildSigningPagePDF(doc, session, outPath); err != nil {
		t.Fatalf("buildSigningPagePDF returned error: %v", err)
	}

	pdfBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected output pdf file to be created: %v", err)
	}
	if bytes.Contains(bytes.ToLower(pdfBytes), []byte("verify")) {
		t.Fatalf("expected verify URL row to be hidden when verify_url is N/A")
	}
}
