package services

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

// makeSignaturePNG рисует крохотный PNG и кодирует его в data URL.
func makeSignaturePNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 120, 40))
	for x := 10; x < 110; x++ {
		img.Set(x, 20, color.RGBA{0, 0, 200, 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// newTestPDF повторяет шрифтовую настройку листа подписания: drawSectionTitle
// внутри drawClientSignature рассчитывает на зарегистрированный «dejavu».
func newTestPDF(t *testing.T) *gofpdf.Fpdf {
	t.Helper()
	fontPath := filepath.Join("..", "..", "assets", "fonts", "DejaVuSans.ttf")
	if _, err := os.Stat(fontPath); err != nil {
		t.Skipf("нет шрифта для теста: %v", err)
	}
	dir := t.TempDir()
	data, _ := os.ReadFile(fontPath)
	_ = os.WriteFile(filepath.Join(dir, "DejaVuSans.ttf"), data, 0o644)

	p := gofpdf.New("P", "mm", "A4", dir)
	p.AddUTF8Font("dejavu", "", "DejaVuSans.ttf")
	p.AddUTF8Font("dejavu", "B", "DejaVuSans.ttf")
	p.AddPage()
	p.SetFont("dejavu", "", 10)
	return p
}

// Нарисованная подпись встраивается в лист подписания и увеличивает объём PDF
// (в него попал PNG) — значит картинка реально добавлена.
func TestDrawClientSignatureEmbedsImage(t *testing.T) {
	sig := makeSignaturePNG(t)

	withSig := newTestPDF(t)
	if err := drawClientSignature(withSig, sig); err != nil {
		t.Fatalf("drawClientSignature: %v", err)
	}
	out := filepath.Join(t.TempDir(), "with.pdf")
	if err := withSig.OutputFileAndClose(out); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(out)

	empty := newTestPDF(t)
	_ = drawClientSignature(empty, "")
	emptyOut := filepath.Join(t.TempDir(), "empty.pdf")
	if err := empty.OutputFileAndClose(emptyOut); err != nil {
		t.Fatal(err)
	}
	emptyInfo, _ := os.Stat(emptyOut)

	if info.Size() <= emptyInfo.Size() {
		t.Fatalf("подпись не встроилась: с подписью %d байт, без %d", info.Size(), emptyInfo.Size())
	}
	t.Logf("PDF с подписью %d байт, без — %d", info.Size(), emptyInfo.Size())
}

// Пустая подпись — не ошибка (ПЭП подтверждается кодом, рисунок опционален).
func TestDrawClientSignatureEmptyIsNoop(t *testing.T) {
	if err := drawClientSignature(newTestPDF(t), ""); err != nil {
		t.Fatalf("пустая подпись должна быть no-op: %v", err)
	}
	if err := drawClientSignature(newTestPDF(t), "   "); err != nil {
		t.Fatalf("пробелы должны быть no-op: %v", err)
	}
}

// Некорректный формат — понятная ошибка, но без паники.
func TestDrawClientSignatureBadFormat(t *testing.T) {
	err := drawClientSignature(newTestPDF(t), "data:image/gif;base64,AAAA")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("ожидалась ошибка формата, получено %v", err)
	}
	if err := drawClientSignature(newTestPDF(t), "data:image/png;base64,не_base64!"); err == nil {
		t.Fatal("ожидалась ошибка декодирования base64")
	}
}
