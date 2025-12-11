package xlsx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Generator — интерфейс (как у docx/pdf)
type Generator interface {
	// GenerateFromTemplate:
	//   templateName — имя файла шаблона, напр. "personal_data.xlsx"
	//   placeholders — карта {{KEY}} -> значение
	//   baseFilename — базовое имя файла без расширения
	//
	// Возвращает относительный путь вида "/excel/personal_data_client_4_20251209_120000.xlsx"
	GenerateFromTemplate(templateName string, placeholders map[string]string, baseFilename string) (string, error)
}

// ExcelGenerator — реализация через "распаковать .xlsx и заменить XML"
type ExcelGenerator struct {
	RootDir      string // куда складывать готовые файлы, напр. "files"
	TemplatesDir string // где лежат шаблоны .xlsx, напр. "assets/templates/xlsx"
}

// NewExcelGenerator — конструктор
func NewExcelGenerator(rootDir, templatesDir string) *ExcelGenerator {
	return &ExcelGenerator{
		RootDir:      filepath.Clean(rootDir),
		TemplatesDir: filepath.Clean(templatesDir),
	}
}

// GenerateFromTemplate — копирует .xlsx-шаблон и заменяет {{KEY}} в xl/*.xml
func (g *ExcelGenerator) GenerateFromTemplate(
	templateName string,
	placeholders map[string]string,
	baseFilename string,
) (string, error) {

	if templateName == "" {
		return "", fmt.Errorf("empty template name")
	}
	if baseFilename == "" {
		baseFilename = fmt.Sprintf("excel_%d", time.Now().Unix())
	}
	baseFilename = filepath.Base(baseFilename)

	// 1. Путь к шаблону
	tmplPath := filepath.Join(g.TemplatesDir, templateName)
	if _, err := os.Stat(tmplPath); err != nil {
		return "", fmt.Errorf("template not found: %s: %w", tmplPath, err)
	}

	// 2. Директория для итоговых Excel-файлов
	excelDir := filepath.Join(g.RootDir, "excel")
	if err := os.MkdirAll(excelDir, 0o755); err != nil {
		return "", fmt.Errorf("create excel dir: %w", err)
	}

	// 3. Итоговый путь
	xlsxFileName := baseFilename + ".xlsx"
	outPath := filepath.Join(excelDir, xlsxFileName)

	if err := g.generateXlsxFromTemplate(tmplPath, outPath, placeholders); err != nil {
		return "", err
	}

	// Возвращаем относительный путь (как у тебя принято)
	rel := "/" + filepath.ToSlash(filepath.Join("excel", xlsxFileName))
	return rel, nil
}

// generateXlsxFromTemplate — как с DOCX: читаем ZIP, патчим XML, пишем новый ZIP
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

		// Меняем только XML внутри xl/ (sharedStrings, sheet*.xml и т.д.)
		if strings.HasPrefix(f.Name, "xl/") && strings.HasSuffix(f.Name, ".xml") {
			text := string(data)
			for k, v := range placeholders {
				ph := "{{" + k + "}}"
				text = strings.ReplaceAll(text, ph, xmlEscape(v))
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

// xmlEscape — то же, что в docx
func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}
