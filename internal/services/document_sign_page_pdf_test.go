package services

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jung-kurt/gofpdf"

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

func TestHashRows_NoDuplicateFirstHalfInPDF(t *testing.T) {
	fontPath, err := resolveSigningFontPath()
	if err != nil {
		t.Fatalf("resolveSigningFontPath: %v", err)
	}
	fontPath, err = ensureSigningFontInTemp(fontPath)
	if err != nil {
		t.Fatalf("ensureSigningFontInTemp: %v", err)
	}

	pdf := gofpdf.New("P", "mm", "A4", "/tmp")
	pdf.SetCompression(false)
	pdf.AddUTF8Font("dejavu", "", filepath.Base(fontPath))
	pdf.AddUTF8Font("dejavu", "B", filepath.Base(fontPath))
	pdf.AddPage()
	pdf.SetFont("dejavu", "", 10)

	hash := "0123456789abcdef0123456789abcdeffedcba9876543210fedcba9876543210"
	line1, line2, ok := splitSHA256(hash)
	if !ok {
		t.Fatalf("expected valid hash")
	}

	drawKeyValue(pdf, "Хэш документа (SHA-256)", line1, 45, 5)
	drawValueContinuation(pdf, line2, 45, 5)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("output pdf: %v", err)
	}
	pdfBytes := buf.Bytes()
	plainCount := bytes.Count(pdfBytes, []byte(line1))
	utf16RawCount := bytes.Count(pdfBytes, utf16BERaw(line1))
	if plainCount+utf16RawCount != 1 {
		t.Fatalf("expected first hash line to be rendered once, got plain=%d utf16raw=%d", plainCount, utf16RawCount)
	}
}

func utf16BERaw(text string) []byte {
	raw := make([]byte, 0, len(text)*2)
	for i := 0; i < len(text); i++ {
		raw = append(raw, 0x00, text[i])
	}
	return raw
}

func TestSplitSHA256(t *testing.T) {
	tests := []struct {
		name  string
		hash  string
		line1 string
		line2 string
		ok    bool
	}{
		{
			name:  "valid hash",
			hash:  "0123456789abcdef0123456789abcdeffedcba9876543210fedcba9876543210",
			line1: "0123456789abcdef0123456789abcdef",
			line2: "fedcba9876543210fedcba9876543210",
			ok:    true,
		},
		{name: "empty", hash: "", ok: false},
		{name: "short", hash: "abc123", ok: false},
		{name: "non hex", hash: "zzzz456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line1, line2, ok := splitSHA256(tc.hash)
			if ok != tc.ok {
				t.Fatalf("ok mismatch: got %v, want %v", ok, tc.ok)
			}
			if line1 != tc.line1 {
				t.Fatalf("line1 mismatch: got %q, want %q", line1, tc.line1)
			}
			if line2 != tc.line2 {
				t.Fatalf("line2 mismatch: got %q, want %q", line2, tc.line2)
			}
		})
	}
}
