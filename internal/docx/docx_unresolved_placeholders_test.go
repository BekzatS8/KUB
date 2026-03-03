package docx

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateDocxAndPDF_UnresolvedPlaceholders(t *testing.T) {
	tmp := t.TempDir()
	templatesDir := filepath.Join(tmp, "templates")
	rootDir := filepath.Join(tmp, "files")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}

	tmplPath := filepath.Join(templatesDir, "bad.docx")
	f, err := os.Create(tmplPath)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	_, _ = w.Write([]byte(`<w:document><w:body>{{UNKNOWN_KEY}}</w:body></w:document>`))
	_ = zw.Close()
	_ = f.Close()

	g := NewDocxGenerator(rootDir, templatesDir, false, "")
	g.SetStrictPlaceholders(true)

	_, _, err = g.GenerateDocxAndPDF("bad.docx", map[string]string{}, "result")
	if err == nil {
		t.Fatalf("expected unresolved_placeholders error")
	}

	u, ok := err.(*UnresolvedPlaceholdersError)
	if !ok {
		t.Fatalf("unexpected error type: %T %v", err, err)
	}
	if len(u.MissingKeys) != 1 || u.MissingKeys[0] != "UNKNOWN_KEY" {
		t.Fatalf("unexpected missing keys: %#v", u.MissingKeys)
	}
}
