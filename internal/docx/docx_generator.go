package docx

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var libreOfficeConvertSem = make(chan struct{}, 2)

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
	StrictPlaceholders bool
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
		StrictPlaceholders: true,
	}
}

func (g *DocxGenerator) SetStrictPlaceholders(strict bool) {
	g.StrictPlaceholders = strict
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
	if err := os.MkdirAll(pdfDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create pdf dir: %w", err)
	}

	// 3. Пути к итоговым файлам
	docxFileName := baseFilename + ".docx"
	pdfFileName := baseFilename + ".pdf"
	tmpTag, err := randomHex(4)
	if err != nil {
		return "", "", fmt.Errorf("tmp tag: %w", err)
	}
	tmpPDFFileName := baseFilename + ".tmp." + tmpTag + ".pdf"

	docxPath := filepath.Join(docxDir, docxFileName)
	pdfPath := filepath.Join(pdfDir, pdfFileName)
	tmpPDFPath := filepath.Join(pdfDir, tmpPDFFileName)

	// 4. Сгенерируем DOCX с подставленными плейсхолдерами
	if err := g.generateDocxFromTemplate(tmplPath, docxPath, placeholders); err != nil {
		return "", "", err
	}
	if g.StrictPlaceholders {
		missingKeys, err := findUnresolvedPlaceholdersInZip(docxPath, "word/")
		if err != nil {
			return "", "", err
		}
		if len(missingKeys) > 0 {
			_ = os.Remove(docxPath)
			return "", "", &UnresolvedPlaceholdersError{TemplateFile: templateName, MissingKeys: missingKeys}
		}
	}

	// 5. Конвертация DOCX → PDF через LibreOffice
	if err := g.convertDocxToPDF(context.Background(), docxPath, pdfDir, tmpPDFFileName); err != nil {
		_ = os.Remove(docxPath)
		_ = os.Remove(pdfPath)
		_ = os.Remove(tmpPDFPath)
		return "", "", err
	}
	if err := os.Rename(tmpPDFPath, pdfPath); err != nil {
		_ = os.Remove(docxPath)
		_ = os.Remove(tmpPDFPath)
		_ = os.Remove(pdfPath)
		return "", "", fmt.Errorf("finalize pdf file: %w", err)
	}

	// 6. Проверим, что PDF появился
	if _, err := os.Stat(pdfPath); err != nil {
		_ = os.Remove(docxPath)
		return "", "", fmt.Errorf("pdf not found after convert: %s: %w", pdfPath, err)
	}
	pdfRel := "/" + filepath.ToSlash(filepath.Join("pdf", pdfFileName))

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
			data, err = replacePlaceholdersRunAware(data, placeholders)
			if err != nil {
				return fmt.Errorf("replace placeholders in %s: %w", f.Name, err)
			}
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
func (g *DocxGenerator) convertDocxToPDF(ctx context.Context, docxPath, outDir, outPDFName string) error {
	binary := g.LibreOfficeBinary
	if binary == "" {
		binary = "libreoffice"
	}
	if !g.LibreOfficeEnabled {
		return fmt.Errorf("libreoffice conversion is disabled")
	}

	if strings.TrimSpace(outPDFName) == "" {
		return fmt.Errorf("output PDF name is required")
	}

	acquireConverter()
	defer releaseConverter()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create pdf out dir: %w", err)
	}
	convertCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	profileID, err := randomHex(16)
	if err != nil {
		return fmt.Errorf("libreoffice profile id: %w", err)
	}
	profileDir := filepath.Join(os.TempDir(), "lo_profile_"+profileID)
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return fmt.Errorf("create libreoffice profile: %w", err)
	}
	defer os.RemoveAll(profileDir)

	profileURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(profileDir)}).String()

	tmpInput := strings.TrimSuffix(outPDFName, ".pdf") + ".docx"
	tmpInputPath := filepath.Join(outDir, tmpInput)
	if err := copyFile(docxPath, tmpInputPath); err != nil {
		return fmt.Errorf("prepare temp docx for convert: %w", err)
	}
	defer os.Remove(tmpInputPath)

	cmd := exec.CommandContext(
		convertCtx,
		binary,
		"--headless",
		"--nologo",
		"--nolockcheck",
		"--norestore",
		"--nodefault",
		"-env:UserInstallation="+profileURI,
		"--convert-to", "pdf",
		"--outdir", outDir,
		tmpInputPath,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(convertCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("libreoffice conversion timeout for %s", filepath.Base(docxPath))
		}
		log.Printf("[docx] libreoffice convert error (binary=%s, docx=%s): %v; stdout=%s; stderr=%s", binary, docxPath, err, stdout.String(), stderr.String())
		return fmt.Errorf("libreoffice conversion failed for %s", filepath.Base(docxPath))
	}
	outPath := filepath.Join(outDir, outPDFName)
	if _, err := os.Stat(outPath); err != nil {
		return fmt.Errorf("converted pdf not found: %s", outPDFName)
	}
	return nil
}

func acquireConverter() {
	libreOfficeConvertSem <- struct{}{}
}

func releaseConverter() {
	select {
	case <-libreOfficeConvertSem:
	default:
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func randomHex(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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

type UnresolvedPlaceholdersError struct {
	TemplateFile string
	MissingKeys  []string
}

func (e *UnresolvedPlaceholdersError) Error() string { return "unresolved_placeholders" }

var placeholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)
var strictPlaceholderPattern = regexp.MustCompile(`\{\{([A-Z0-9_]+)\}\}`)

type textNode struct {
	start      int
	end        int
	innerStart int
	innerEnd   int
}

func replacePlaceholdersRunAware(data []byte, placeholders map[string]string) ([]byte, error) {
	nodes, stream := extractTextNodes(data)
	if len(nodes) == 0 || len(placeholders) == 0 {
		return data, nil
	}

	owners := make([]int, len(stream))
	for i, n := range nodes {
		for p := n.start; p < n.end; p++ {
			owners[p] = i
		}
	}

	result := make([]string, len(nodes))
	matches := strictPlaceholderPattern.FindAllStringSubmatchIndex(stream, -1)
	pos := 0
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		s, e := m[0], m[1]
		keyStart, keyEnd := m[2], m[3]
		key := stream[keyStart:keyEnd]
		repl, ok := placeholders[key]
		if !ok {
			continue
		}
		for pos < s {
			nodeIx := owners[pos]
			result[nodeIx] += string(stream[pos])
			pos++
		}
		firstNodeIx := owners[s]
		result[firstNodeIx] += repl
		pos = e
	}
	for pos < len(stream) {
		nodeIx := owners[pos]
		result[nodeIx] += string(stream[pos])
		pos++
	}

	var out bytes.Buffer
	last := 0
	for i, node := range nodes {
		out.Write(data[last:node.innerStart])
		out.WriteString(xmlEscape(result[i]))
		last = node.innerEnd
	}
	out.Write(data[last:])
	return out.Bytes(), nil
}

var wTextNodePattern = regexp.MustCompile(`(?s)<w:t\b[^>]*>(.*?)</w:t>`)

func extractTextNodes(data []byte) ([]textNode, string) {
	var nodes []textNode
	var stream strings.Builder
	for _, m := range wTextNodePattern.FindAllSubmatchIndex(data, -1) {
		if len(m) < 4 {
			continue
		}
		innerStart, innerEnd := m[2], m[3]
		text := html.UnescapeString(string(data[innerStart:innerEnd]))
		nodes = append(nodes, textNode{
			start:      stream.Len(),
			end:        stream.Len() + len(text),
			innerStart: innerStart,
			innerEnd:   innerEnd,
		})
		stream.WriteString(text)
	}
	return nodes, stream.String()
}

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
		_, textStream := extractTextNodes(b)
		for _, m := range placeholderPattern.FindAllStringSubmatch(textStream, -1) {
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
