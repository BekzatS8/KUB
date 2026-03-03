package xlsx

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Generator — интерфейс (как у docx/pdf)
type Generator interface {
	GenerateFromTemplate(templateName string, placeholders map[string]string, baseFilename string) (string, error)
	GenerateFromTemplateAndPDF(templateName string, placeholders map[string]string, baseFilename string) (string, string, error)
}

type ExcelGenerator struct {
	RootDir              string
	TemplatesDir         string
	LibreOfficeBinary    string
	LibreOfficeEnabled   bool
	PDFProvider          string // libreoffice|external
	ExternalPDFConverter string
	StrictPlaceholders   bool
}

func NewExcelGenerator(rootDir, templatesDir string, enableLibreOffice bool, libreOfficeBinary string) *ExcelGenerator {
	if libreOfficeBinary == "" {
		libreOfficeBinary = "libreoffice"
	}
	return &ExcelGenerator{RootDir: filepath.Clean(rootDir), TemplatesDir: filepath.Clean(templatesDir), LibreOfficeEnabled: enableLibreOffice, LibreOfficeBinary: libreOfficeBinary, PDFProvider: "libreoffice", StrictPlaceholders: true}
}

func (g *ExcelGenerator) SetStrictPlaceholders(strict bool) {
	g.StrictPlaceholders = strict
}

func (g *ExcelGenerator) GenerateFromTemplate(templateName string, placeholders map[string]string, baseFilename string) (string, error) {
	xlsxRel, _, err := g.GenerateFromTemplateAndPDF(templateName, placeholders, baseFilename)
	return xlsxRel, err
}

func (g *ExcelGenerator) GenerateFromTemplateAndPDF(templateName string, placeholders map[string]string, baseFilename string) (string, string, error) {
	if templateName == "" {
		return "", "", fmt.Errorf("template not found: empty template name")
	}
	if baseFilename == "" {
		baseFilename = fmt.Sprintf("excel_%d", time.Now().Unix())
	}
	baseFilename = filepath.Base(baseFilename)
	tmplPath := filepath.Join(g.TemplatesDir, templateName)
	if _, err := os.Stat(tmplPath); err != nil {
		return "", "", fmt.Errorf("template not found: %s: %w", tmplPath, err)
	}
	excelDir := filepath.Join(g.RootDir, "excel")
	pdfDir := filepath.Join(g.RootDir, "pdf")
	if err := os.MkdirAll(excelDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create excel dir: %w", err)
	}
	if err := os.MkdirAll(pdfDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create pdf dir: %w", err)
	}

	xlsxName := baseFilename + ".xlsx"
	xlsxPath := filepath.Join(excelDir, xlsxName)
	if err := g.generateXlsxFromTemplate(tmplPath, xlsxPath, placeholders); err != nil {
		return "", "", err
	}
	if g.StrictPlaceholders {
		missingKeys, err := findUnresolvedPlaceholdersInZip(xlsxPath, "xl/")
		if err != nil {
			return "", "", err
		}
		if len(missingKeys) > 0 {
			_ = os.Remove(xlsxPath)
			return "", "", &UnresolvedPlaceholdersError{TemplateFile: templateName, MissingKeys: missingKeys}
		}
	}
	xlsxRel := "/" + filepath.ToSlash(filepath.Join("excel", xlsxName))

	if strings.EqualFold(g.PDFProvider, "external") {
		return xlsxRel, "", fmt.Errorf("pdf_conversion_disabled: external converter is not configured")
	}
	if !g.LibreOfficeEnabled {
		return xlsxRel, "", fmt.Errorf("pdf_conversion_disabled: libreoffice conversion is disabled")
	}
	pdfName := baseFilename + ".pdf"
	if err := g.convertXlsxToPDF(context.Background(), xlsxPath, pdfDir, pdfName); err != nil {
		return "", "", fmt.Errorf("pdf_conversion_failed: %w", err)
	}
	pdfRel := "/" + filepath.ToSlash(filepath.Join("pdf", pdfName))
	return xlsxRel, pdfRel, nil
}

func (g *ExcelGenerator) convertXlsxToPDF(ctx context.Context, xlsxPath, outDir, outPDFName string) error {
	cmd := exec.CommandContext(ctx, g.LibreOfficeBinary, "--headless", "--convert-to", "pdf", "--outdir", outDir, xlsxPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("libreoffice conversion failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	found := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(xlsxPath), filepath.Ext(xlsxPath))+".pdf")
	if _, err := os.Stat(found); err != nil {
		return fmt.Errorf("converted pdf not found: %s", found)
	}
	finalPath := filepath.Join(outDir, outPDFName)
	if found != finalPath {
		if err := os.Rename(found, finalPath); err != nil {
			return fmt.Errorf("rename converted pdf: %w", err)
		}
	}
	return nil
}

func (g *ExcelGenerator) generateXlsxFromTemplate(templatePath, outPath string, placeholders map[string]string) error {
	r, err := zip.OpenReader(templatePath)
	if err != nil {
		return fmt.Errorf("open template xlsx: %w", err)
	}
	defer r.Close()
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}
	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create out xlsx: %w", err)
	}
	defer outFile.Close()
	zw := zip.NewWriter(outFile)
	defer zw.Close()
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open file inside xlsx: %w", err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("read file inside xlsx: %w", err)
		}
		if strings.HasPrefix(f.Name, "xl/") && strings.HasSuffix(f.Name, ".xml") {
			text := string(data)
			for k, v := range placeholders {
				text = strings.ReplaceAll(text, "{{"+k+"}}", xmlEscape(v))
			}
			data = []byte(text)
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			return fmt.Errorf("create file in new xlsx: %w", err)
		}
		if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
			return fmt.Errorf("write file in new xlsx: %w", err)
		}
	}
	return nil
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return replacer.Replace(s)
}

type UnresolvedPlaceholdersError struct {
	TemplateFile string
	MissingKeys  []string
}

func (e *UnresolvedPlaceholdersError) Error() string { return "unresolved_placeholders" }

var placeholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)

func findUnresolvedPlaceholdersInZip(zipPath, prefix string) ([]string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open generated file: %w", err)
	}
	defer r.Close()
	set := map[string]struct{}{}
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, prefix) || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open generated xml: %w", err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read generated xml: %w", err)
		}
		for _, m := range placeholderPattern.FindAllStringSubmatch(string(b), -1) {
			if len(m) > 1 {
				set[m[1]] = struct{}{}
			}
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}
