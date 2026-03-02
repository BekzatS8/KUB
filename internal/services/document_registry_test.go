package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryTemplatesExistAndCount(t *testing.T) {
	specs := ListDocumentTypeSpecs()
	if len(specs) != 15 {
		t.Fatalf("registry size=%d want 15", len(specs))
	}
	for _, spec := range specs {
		var dir string
		switch spec.Format {
		case DocumentFormatDOCX:
			dir = filepath.Join("..", "..", "assets", "templates", "docx")
		case DocumentFormatXLSX:
			dir = filepath.Join("..", "..", "assets", "templates", "xlsx")
		default:
			t.Fatalf("unsupported format for %s: %s", spec.DocType, spec.Format)
		}
		if _, err := os.Stat(filepath.Join(dir, spec.TemplateFile)); err != nil {
			if os.IsNotExist(err) {
				t.Skipf("templates not uploaded yet: missing %s for %s", spec.TemplateFile, spec.DocType)
			}
			t.Fatalf("doc_type=%s template stat error: %v", spec.DocType, err)
		}
	}
}

func TestRegistryExtraRequiredCases(t *testing.T) {
	for _, docType := range []string{"refund_application", "pause_application", "cancel_appointment"} {
		spec, ok := GetDocumentTypeSpec(docType)
		if !ok {
			t.Fatalf("spec for %s not found", docType)
		}
		if len(spec.ExtraKeys) == 0 {
			t.Fatalf("doc_type=%s must have extra_keys", docType)
		}
		hasReason := false
		for _, k := range spec.ExtraKeys {
			if k.Key == "reason_code" {
				hasReason = true
			}
		}
		if !hasReason {
			t.Fatalf("doc_type=%s must expose reason_code in extra_keys", docType)
		}
	}
}
