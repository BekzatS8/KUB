package docx

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateDocxFromTemplate_PreservesCyrillicAndReplacesSplitPlaceholder(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "template.docx")
	outPath := filepath.Join(tmpDir, "out.docx")

	const documentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>ТОО «KUB GROUP»</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:t>Договор № </w:t></w:r>
      <w:r><w:t>{{CONTR</w:t></w:r>
      <w:r><w:t>ACT_NUMBER}}</w:t></w:r>
      <w:r><w:t> от 15 марта 2026 г.</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`

	writeDocxTemplate(t, templatePath, map[string]string{
		"[Content_Types].xml":          `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
		"_rels/.rels":                  `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`,
		"word/document.xml":            documentXML,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`,
		"word/styles.xml":              `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"></w:styles>`,
		"docProps/core.xml":            `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"></cp:coreProperties>`,
		"docProps/app.xml":             `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"></Properties>`,
	})

	g := NewDocxGenerator(tmpDir, tmpDir, false, "")
	err := g.generateDocxFromTemplate(templatePath, outPath, map[string]string{
		"CONTRACT_NUMBER": "KZ-42/2026",
	})
	if err != nil {
		t.Fatalf("generate docx from template: %v", err)
	}

	gotXML := readDocxEntry(t, outPath, "word/document.xml")
	if strings.Contains(gotXML, "{{CONTRACT_NUMBER}}") {
		t.Fatalf("placeholder was not replaced: %s", gotXML)
	}
	if !strings.Contains(gotXML, "KZ-42/2026") {
		t.Fatalf("replacement value not found: %s", gotXML)
	}
	if !strings.Contains(gotXML, "ТОО") {
		t.Fatalf("cyrillic text was not preserved: %s", gotXML)
	}
	if !strings.Contains(gotXML, "марта") {
		t.Fatalf("cyrillic month was not preserved: %s", gotXML)
	}
	if strings.Contains(gotXML, "Ð") {
		t.Fatalf("mojibake detected in generated xml: %s", gotXML)
	}
}

func writeDocxTemplate(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create template entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write template entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close template zip: %v", err)
	}
}

func readDocxEntry(t *testing.T, path, name string) string {
	t.Helper()

	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open docx: %v", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", name, err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read entry %s: %v", name, err)
		}
		return string(b)
	}

	t.Fatalf("entry %s not found", name)
	return ""
}
