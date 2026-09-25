package storage

import "testing"

func TestValidateCompanyPrefix(t *testing.T) {
	for _, ok := range []string{"", "kub", "kub-visa", "company2", "a"} {
		if err := ValidateCompanyPrefix(ok); err != nil {
			t.Errorf("%q must be valid: %v", ok, err)
		}
	}
	bad := []string{
		"KUB",      // заглавные — ключи S3 чувствительны к регистру, путаница
		"kub/visa", // слэш сломал бы структуру
		"-kub",     // должен начинаться с буквы или цифры
		"куб",      // кириллица в ключах бакета
		"kub visa", // пробел
		"pdf",      // совпадает с системной папкой в корне
		"drive",    // тоже
		"versions", // тоже
		"messages", // тоже — старые вложения чата
		"a234567890123456789012345678901234567890123456789012345678901234", // 64 символа
	}
	for _, p := range bad {
		if err := ValidateCompanyPrefix(p); err == nil {
			t.Errorf("%q must be rejected", p)
		}
	}
}

func TestNormalizeCompanyPrefix(t *testing.T) {
	if got := NormalizeCompanyPrefix("  /kub/ "); got != "kub" {
		t.Fatalf("want kub, got %q", got)
	}
}

// TestPrefixedKeyKeepsLegacyKeysIntact — без префикса ключ не меняется ни на
// байт: инсталляция, не включившая разделение, читает свои файлы как раньше.
func TestPrefixedKeyKeepsLegacyKeysIntact(t *testing.T) {
	for _, k := range []string{"pdf/a.pdf", "/pdf/a.pdf", "clients/1/x.jpg"} {
		if got := prefixedKey("", k); got != k {
			t.Errorf("no prefix must keep %q as is, got %q", k, got)
		}
	}
	cases := map[string]string{
		"pdf/a.pdf":           "kub/pdf/a.pdf",
		"/pdf/a.pdf":          "kub/pdf/a.pdf", // без «kub//pdf»
		"drive/2026/09/f.txt": "kub/drive/2026/09/f.txt",
	}
	for in, want := range cases {
		if got := prefixedKey("kub", in); got != want {
			t.Errorf("%q: want %q, got %q", in, want, got)
		}
	}
}

// TestPlanPrefixMove — что переносится, а что нет. Главное правило: трогаем
// только системные папки; неизвестное в корне может быть чужим.
func TestPlanPrefixMove(t *testing.T) {
	folders := map[string]bool{}
	for f := range LegacyTopLevelFolders {
		folders[f] = true
	}
	cases := []struct {
		key     string
		move    bool
		already bool
		dest    string
		top     string
	}{
		{key: "pdf/contract_deal_12.pdf", move: true, dest: "kub/pdf/contract_deal_12.pdf", top: "pdf"},
		{key: "/pdf/legacy.pdf", move: true, dest: "kub/pdf/legacy.pdf", top: "pdf"},
		{key: "clients/5/passport/a.jpg", move: true, dest: "kub/clients/5/passport/a.jpg", top: "clients"},
		{key: "drive-previews/5.pdf", move: true, dest: "kub/drive-previews/5.pdf", top: "drive-previews"},
		// Старые вложения чата: папка есть в боевом бакете, в текущем коде не пишется.
		{key: "messages/1734000000_scan.pdf", move: true, dest: "kub/messages/1734000000_scan.pdf", top: "messages"},
		{key: "kub/pdf/already.pdf", already: true, top: "kub"},
		{key: "company2/pdf/a.pdf", top: "company2"},      // чужая компания — не трогать
		{key: "backup-2025/dump.sql", top: "backup-2025"}, // неизвестная папка — не трогать
		{key: "readme.txt", top: ""},                      // файл в корне — не трогать
	}
	for _, c := range cases {
		d := planPrefixMove(c.key, "kub", folders)
		if d.Move != c.move || d.Already != c.already || d.Dest != c.dest || d.Top != c.top {
			t.Errorf("%q: got %+v, want move=%v already=%v dest=%q top=%q", c.key, d, c.move, c.already, c.dest, c.top)
		}
	}
}
