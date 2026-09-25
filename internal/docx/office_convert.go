package docx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ConvertOfficeToPDF конвертирует офисный документ любого формата, который
// понимает LibreOffice (doc, docx, xls, xlsx, ppt, pptx, odt, rtf…), в PDF.
//
// В отличие от convertDocxToPDF не привязан к генератору договоров: нужен для
// предпросмотра произвольных файлов в хранилище. Использует тот же семафор
// libreOfficeConvertSem, чтобы предпросмотры не конкурировали с генерацией
// документов за процессы soffice, и ту же изоляцию профиля — параллельные
// запуски LibreOffice с общим профилем блокируют друг друга.
//
// Возвращает путь к PDF внутри outDir; вызывающий удаляет outDir сам.
func ConvertOfficeToPDF(ctx context.Context, binary, inputPath, outDir string) (string, error) {
	if strings.TrimSpace(binary) == "" {
		binary = "libreoffice"
	}

	acquireConverter()
	defer releaseConverter()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create pdf out dir: %w", err)
	}
	convertCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	profileID, err := randomHex(16)
	if err != nil {
		return "", fmt.Errorf("libreoffice profile id: %w", err)
	}
	profileDir := filepath.Join(os.TempDir(), "lo_profile_"+profileID)
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return "", fmt.Errorf("create libreoffice profile: %w", err)
	}
	defer os.RemoveAll(profileDir)
	profileURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(profileDir)}).String()

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
		inputPath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(convertCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("libreoffice conversion timeout for %s", filepath.Base(inputPath))
		}
		log.Printf("[office] libreoffice convert error (binary=%s, input=%s): %v; stdout=%s; stderr=%s",
			binary, filepath.Base(inputPath), err, stdout.String(), stderr.String())
		return "", fmt.Errorf("libreoffice conversion failed for %s", filepath.Base(inputPath))
	}

	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outPath := filepath.Join(outDir, base+".pdf")
	if _, err := os.Stat(outPath); err != nil {
		return "", fmt.Errorf("converted pdf not found for %s", filepath.Base(inputPath))
	}
	return outPath, nil
}
