package services

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Signature stamp positions on the contract PDF (PDF points, origin = lower-left of page).
// A4 page = 595 x 842 pt. Tune these after printing a test document.
const (
	sigStampX      = 350.0 // left edge of signature image
	sigStampY      = 65.0  // bottom edge of signature image
	sigStampWidth  = 150.0 // image width in points
	sigStampHeight = 45.0  // image height in points
)

// SignaturePosition describes where to stamp the signature on one page.
type SignaturePosition struct {
	Page          int
	X, Y          float64 // lower-left corner of the image in PDF points
	Width, Height float64
}

// DefaultContractSignaturePositions are the three client-signature fields in the
// standard contract template (section 14, Appendix 1, Appendix 2).
var DefaultContractSignaturePositions = []SignaturePosition{
	{Page: 8, X: sigStampX, Y: sigStampY, Width: sigStampWidth, Height: sigStampHeight},
	{Page: 10, X: sigStampX, Y: sigStampY, Width: sigStampWidth, Height: sigStampHeight},
	{Page: 12, X: sigStampX, Y: sigStampY, Width: sigStampWidth, Height: sigStampHeight},
}

// StampSignatureOnPDF overlays the PNG image at signatureImagePath onto the pages
// defined by DefaultContractSignaturePositions in the PDF at pdfPath, writing the
// result to outputPath (may be the same as pdfPath for in-place update).
//
// Requires the pdfcpu binary in PATH. Returns ErrPDFCPUMissing when it is absent.
func StampSignatureOnPDF(pdfPath, signatureImagePath, outputPath string) error {
	if _, err := os.Stat(signatureImagePath); err != nil {
		return fmt.Errorf("[pdf_stamp] signature image not found: %w", err)
	}
	if _, err := os.Stat(pdfPath); err != nil {
		return fmt.Errorf("[pdf_stamp] pdf not found: %w", err)
	}

	pdfcpuPath, err := exec.LookPath("pdfcpu")
	if err != nil {
		return ErrPDFCPUMissing
	}

	// Build comma-separated page list from positions (all use the same position config).
	pages := buildPageList(DefaultContractSignaturePositions)

	// Use positions from the first entry; all entries share the same X/Y/W/H.
	pos := DefaultContractSignaturePositions[0]
	desc := fmt.Sprintf(
		"sc:abs:%.0f %.0f, pos:bl, off:%.0f %.0f, rot:0, opacity:1",
		pos.Width, pos.Height, pos.X, pos.Y,
	)

	// pdfcpu writes to outputPath; for in-place updates use a temp file then rename.
	actualOut := outputPath
	inPlace := filepath.Clean(pdfPath) == filepath.Clean(outputPath)
	if inPlace {
		actualOut = outputPath + ".stamp_tmp"
	}

	if err := os.MkdirAll(filepath.Dir(actualOut), 0o755); err != nil {
		return fmt.Errorf("[pdf_stamp] create output dir: %w", err)
	}

	// pdfcpu stamp add -mode image -pages "8,10,12" -- "desc" img.png input.pdf output.pdf
	cmd := exec.Command(pdfcpuPath,
		"stamp", "add",
		"-mode", "image",
		"-pages", pages,
		"--", desc,
		signatureImagePath,
		pdfPath,
		actualOut,
	)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		_ = os.Remove(actualOut)
		return fmt.Errorf("[pdf_stamp] pdfcpu stamp: %v (%s)", runErr, strings.TrimSpace(string(out)))
	}

	if inPlace {
		if renameErr := os.Rename(actualOut, outputPath); renameErr != nil {
			_ = os.Remove(actualOut)
			return fmt.Errorf("[pdf_stamp] replace original: %w", renameErr)
		}
	}

	log.Printf("[pdf_stamp] stamped pages=%s pdf=%s", pages, filepath.Base(outputPath))
	return nil
}

func buildPageList(positions []SignaturePosition) string {
	seen := make(map[int]struct{}, len(positions))
	parts := make([]string, 0, len(positions))
	for _, p := range positions {
		if _, ok := seen[p.Page]; ok {
			continue
		}
		seen[p.Page] = struct{}{}
		parts = append(parts, fmt.Sprintf("%d", p.Page))
	}
	return strings.Join(parts, ",")
}
