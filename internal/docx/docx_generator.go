package docx

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Generator — интерфейс (можно мокать в тестах)
type Generator interface {
	// GenerateDocxAndPDF:
	//  templateName  — имя файла шаблона, напр. "contract_full.docx"
	//  placeholders  — карта {{KEY}} -> значение
	//  baseFilename  — базовое имя файла без расширения
	//
	// Возвращает:
	//  docxRelPath — относительный путь к DOCX (напр. "/docx/contract_full_client_4_deal_2_20251207_140501.docx")
	//  pdfRelPath  — относительный путь к PDF  (напр. "/pdf/contract_full_client_4_deal_2_20251207_140501.pdf")
	GenerateDocxAndPDF(templateName string, placeholders map[string]string, baseFilename string) (string, string, error)

	// Вспомогательный метод: только PDF (для обратной совместимости)
	GeneratePDF(templateName string, placeholders map[string]string, baseFilename string) (string, error)
}

// DocxGenerator — реализация через LibreOffice
type DocxGenerator struct {
	RootDir            string // корень, куда складывать готовые файлы, напр. "files"
	TemplatesDir       string // где лежат шаблоны .docx, напр. "assets/templates/docx"
	LibreOfficeBinary  string // путь к soffice, напр. "libreoffice" или "soffice"
	LibreOfficeEnabled bool
}

// NewDocxGenerator — конструктор
func NewDocxGenerator(rootDir, templatesDir string, enableLibreOffice bool, libreOfficeBinary string) *DocxGenerator {
	if libreOfficeBinary == "" {
		libreOfficeBinary = "libreoffice" // по умолчанию ищем в PATH
	}

	if templatesDir == "" {
		templatesDir = "assets/templates/docx"
	}

	return &DocxGenerator{
		RootDir:            filepath.Clean(rootDir),
		TemplatesDir:       filepath.Clean(templatesDir),
		LibreOfficeBinary:  libreOfficeBinary,
		LibreOfficeEnabled: enableLibreOffice,
	}
}

// GenerateDocxAndPDF — DOCX-шаблон + плейсхолдеры → DOCX в /docx → PDF в /pdf (если включен LibreOffice)
func (g *DocxGenerator) GenerateDocxAndPDF(
	templateName string,
	placeholders map[string]string,
	baseFilename string,
) (string, string, error) {

	if templateName == "" {
		return "", "", fmt.Errorf("empty template name")
	}
	if baseFilename == "" {
		baseFilename = fmt.Sprintf("doc_%d", time.Now().Unix())
	}

	baseFilename = filepath.Base(baseFilename)

	// 1. Путь к шаблону
	tmplPath := filepath.Join(g.TemplatesDir, templateName)
	if _, err := os.Stat(tmplPath); err != nil {
		return "", "", fmt.Errorf("template not found: %s: %w", tmplPath, err)
	}

	// 2. Готовим директории
	docxDir := filepath.Join(g.RootDir, "docx")
	pdfDir := filepath.Join(g.RootDir, "pdf")

	if err := os.MkdirAll(docxDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create docx dir: %w", err)
	}
	if g.LibreOfficeEnabled {
		if err := os.MkdirAll(pdfDir, 0o755); err != nil {
			return "", "", fmt.Errorf("create pdf dir: %w", err)
		}
	}

	// 3. Пути к итоговым файлам
	docxFileName := baseFilename + ".docx"
	pdfFileName := baseFilename + ".pdf"

	docxPath := filepath.Join(docxDir, docxFileName)
	pdfPath := filepath.Join(pdfDir, pdfFileName)

	// 4. Сгенерируем DOCX с подставленными плейсхолдерами
	if err := g.generateDocxFromTemplate(tmplPath, docxPath, placeholders); err != nil {
		return "", "", err
	}

	var pdfRel string
	// 5. Конвертация DOCX → PDF через LibreOffice (если включено)
	if g.LibreOfficeEnabled {
		if err := g.convertDocxToPDF(context.Background(), docxPath, pdfDir); err != nil {
			return "", "", err
		}

		// 6. Проверим, что PDF появился
		if _, err := os.Stat(pdfPath); err != nil {
			return "", "", fmt.Errorf("pdf not found after convert: %s: %w", pdfPath, err)
		}
		pdfRel = "/" + filepath.ToSlash(filepath.Join("pdf", pdfFileName))
	}

	// DOCX мы теперь НЕ удаляем — он нужен как Word-версия

	// 7. Возвращаем относительные пути (как обычно отдаёшь в API)
	docxRel := "/" + filepath.ToSlash(filepath.Join("docx", docxFileName))

	return docxRel, pdfRel, nil
}

// GeneratePDF — обёртка над GenerateDocxAndPDF (для старого кода)
func (g *DocxGenerator) GeneratePDF(
	templateName string,
	placeholders map[string]string,
	baseFilename string,
) (string, error) {
	_, pdfRel, err := g.GenerateDocxAndPDF(templateName, placeholders, baseFilename)
	return pdfRel, err
}

// generateDocxFromTemplate — копирует docx и заменяет {{KEY}} в word/*.xml
func (g *DocxGenerator) generateDocxFromTemplate(templatePath, outPath string, placeholders map[string]string) error {
	r, err := zip.OpenReader(templatePath)
	if err != nil {
		return fmt.Errorf("open template docx: %w", err)
	}
	defer r.Close()

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create out docx: %w", err)
	}
	defer outFile.Close()

	zw := zip.NewWriter(outFile)
	defer zw.Close()

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open file inside docx: %w", err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("read file inside docx: %w", err)
		}

		// Заменяем только в XML внутри папки word/ (document.xml, header*.xml, footer*.xml и т.п.)
		if strings.HasPrefix(f.Name, "word/") && strings.HasSuffix(f.Name, ".xml") {
			text := string(data)
			for k, v := range placeholders {
				ph := "{{" + k + "}}"
				text = strings.ReplaceAll(text, ph, xmlEscape(v))
			}
			data = []byte(text)
		}

		w, err := zw.Create(f.Name)
		if err != nil {
			return fmt.Errorf("create file in new docx: %w", err)
		}
		if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
			return fmt.Errorf("write file in new docx: %w", err)
		}
	}

	return nil
}

// convertDocxToPDF — запускает LibreOffice в headless-режиме
func (g *DocxGenerator) convertDocxToPDF(ctx context.Context, docxPath, outDir string) error {
	binary := g.LibreOfficeBinary
	if binary == "" {
		binary = "libreoffice"
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create pdf out dir: %w", err)
	}

	cmd := exec.CommandContext(
		ctx,
		binary,
		"--headless",
		"--convert-to", "pdf",
		"--outdir", outDir,
		docxPath,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[docx] libreoffice convert error (binary=%s, docx=%s): %v; stdout=%s; stderr=%s", binary, docxPath, err, stdout.String(), stderr.String())
		return fmt.Errorf("libreoffice conversion failed")
	}
	return nil
}

// xmlEscape — минимальное экранирование спецсимволов для XML
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
