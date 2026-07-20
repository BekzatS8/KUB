package handlers

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

// buildSimpleXLSX собирает минимальный валидный .xlsx (одна страница) из
// заголовков и строк. Без внешних библиотек — руками пишем OOXML с inline-
// строками (t="inlineStr"), поэтому sharedStrings не нужен. Excel/LibreOffice/
// Google Sheets открывают такой файл штатно.
func buildSimpleXLSX(sheetName string, header []string, rows [][]string) ([]byte, error) {
	if strings.TrimSpace(sheetName) == "" {
		sheetName = "Sheet1"
	}

	var sheet bytes.Buffer
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)

	writeRow := func(cells []string) {
		sheet.WriteString("<row>")
		for _, c := range cells {
			sheet.WriteString(`<c t="inlineStr"><is><t xml:space="preserve">`)
			sheet.WriteString(xlsxEscape(c))
			sheet.WriteString(`</t></is></c>`)
		}
		sheet.WriteString("</row>")
	}

	if len(header) > 0 {
		writeRow(header)
	}
	for _, r := range rows {
		writeRow(r)
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
		`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
		`</Types>`

	rootRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
		`</Relationships>`

	workbook := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		`<sheets><sheet name="` + xlsxEscape(clampSheetName(sheetName)) + `" sheetId="1" r:id="rId1"/></sheets>` +
		`</workbook>`

	workbookRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
		`</Relationships>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := []struct{ name, body string }{
		{"[Content_Types].xml", contentTypes},
		{"_rels/.rels", rootRels},
		{"xl/workbook.xml", workbook},
		{"xl/_rels/workbook.xml.rels", workbookRels},
		{"xl/worksheets/sheet1.xml", sheet.String()},
	}
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, fmt.Errorf("xlsx zip create %s: %w", f.name, err)
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			return nil, fmt.Errorf("xlsx zip write %s: %w", f.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("xlsx zip close: %w", err)
	}
	return buf.Bytes(), nil
}

func xlsxEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	// управляющие символы недопустимы в XML — вычищаем
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return r
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, s)
}

// clampSheetName обрезает имя листа до 31 символа и убирает запрещённые символы.
func clampSheetName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case ':', '\\', '/', '?', '*', '[', ']':
			return ' '
		}
		return r
	}, name)
	runes := []rune(name)
	if len(runes) > 31 {
		runes = runes[:31]
	}
	return string(runes)
}
