package handlers

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestBuildSimpleXLSX(t *testing.T) {
	data, err := buildSimpleXLSX("Отчёт за июль", []string{"Дата", "Клиент"}, [][]string{
		{"01.07", "Ахметов & Со"},
		{"02.07", "Иванов <ИП>"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// валидный zip
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("не валидный zip: %v", err)
	}
	need := map[string]bool{
		"[Content_Types].xml": false, "_rels/.rels": false,
		"xl/workbook.xml": false, "xl/_rels/workbook.xml.rels": false,
		"xl/worksheets/sheet1.xml": false,
	}
	var sheet string
	for _, f := range zr.File {
		if _, ok := need[f.Name]; ok {
			need[f.Name] = true
		}
		if f.Name == "xl/worksheets/sheet1.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			sheet = string(b)
		}
	}
	for name, present := range need {
		if !present {
			t.Errorf("в xlsx нет обязательной части %q", name)
		}
	}
	// данные и экранирование
	if !strings.Contains(sheet, "Ахметов &amp; Со") {
		t.Errorf("амперсанд не экранирован: %s", sheet)
	}
	if !strings.Contains(sheet, "Иванов &lt;ИП&gt;") {
		t.Errorf("угловые скобки не экранированы: %s", sheet)
	}
	if !strings.Contains(sheet, "Дата") || !strings.Contains(sheet, "Клиент") {
		t.Error("заголовки не попали в лист")
	}
}

func TestClampSheetName(t *testing.T) {
	if got := clampSheetName("A:B/C?D*[E]"); strings.ContainsAny(got, `:\/?*[]`) {
		t.Errorf("запрещённые символы не убраны: %q", got)
	}
	long := strings.Repeat("я", 40)
	if runes := []rune(clampSheetName(long)); len(runes) != 31 {
		t.Errorf("имя листа не обрезано до 31: %d", len(runes))
	}
}
