package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	"turcompany/internal/authz"
)

func newLinkTestService(now time.Time) *DriveService {
	s := NewDriveService(nil, nil, DriveConfig{LinkSecret: DriveLinkSecret([]byte("jwt-secret-for-tests-32-bytes-long!"))})
	s.now = func() time.Time { return now }
	return s
}

func TestDriveLinkRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	s := newLinkTestService(now)
	token := s.signLink(driveLinkClaims{NodeID: 42, UserID: 7, Expires: now.Add(time.Hour).Unix(), Variant: DriveVariantPDF, Inline: true})

	claims, err := s.verifyLink(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.NodeID != 42 || claims.UserID != 7 || claims.Variant != DriveVariantPDF || !claims.Inline {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

// TestDriveLinkRejectsTampering — подмена id файла в ссылке не должна давать
// доступ к чужому файлу: подпись перестаёт сходиться.
func TestDriveLinkRejectsTampering(t *testing.T) {
	now := time.Now()
	s := newLinkTestService(now)
	token := s.signLink(driveLinkClaims{NodeID: 42, UserID: 7, Expires: now.Add(time.Hour).Unix(), Variant: DriveVariantOriginal})

	payload, sig, _ := strings.Cut(token, ".")
	forged := s.signLink(driveLinkClaims{NodeID: 43, UserID: 7, Expires: now.Add(time.Hour).Unix(), Variant: DriveVariantOriginal})
	forgedPayload, _, _ := strings.Cut(forged, ".")

	if _, err := s.verifyLink(forgedPayload + "." + sig); !errors.Is(err, ErrDriveLinkInvalid) {
		t.Fatalf("payload swap must be rejected, got %v", err)
	}
	if _, err := s.verifyLink(payload + ".AAAA"); !errors.Is(err, ErrDriveLinkInvalid) {
		t.Fatalf("bad signature must be rejected, got %v", err)
	}
	if _, err := s.verifyLink("garbage"); !errors.Is(err, ErrDriveLinkInvalid) {
		t.Fatalf("garbage must be rejected, got %v", err)
	}

	// Другой секрет — другая подпись.
	other := NewDriveService(nil, nil, DriveConfig{LinkSecret: DriveLinkSecret([]byte("another-secret"))})
	if _, err := other.verifyLink(token); !errors.Is(err, ErrDriveLinkInvalid) {
		t.Fatalf("token from another secret must be rejected, got %v", err)
	}
}

func TestDriveLinkExpires(t *testing.T) {
	now := time.Now()
	s := newLinkTestService(now)
	token := s.signLink(driveLinkClaims{NodeID: 1, UserID: 1, Expires: now.Add(-time.Second).Unix(), Variant: DriveVariantOriginal})
	if _, err := s.verifyLink(token); !errors.Is(err, ErrDriveLinkExpired) {
		t.Fatalf("expected ErrDriveLinkExpired, got %v", err)
	}
}

// TestDriveLinkSecretIsNotJWTSecret — ключ подписи ссылок производный: утечка
// ссылки не должна раскрывать ничего о JWT-секрете, и наоборот.
func TestDriveLinkSecretIsNotJWTSecret(t *testing.T) {
	jwt := []byte("jwt-secret-for-tests-32-bytes-long!")
	if string(DriveLinkSecret(jwt)) == string(jwt) {
		t.Fatal("link secret must differ from jwt secret")
	}
}

func TestNormalizeDriveName(t *testing.T) {
	good := map[string]string{
		"  Договоры 2026  ": "Договоры 2026",
		"отчёт.v2.xlsx":     "отчёт.v2.xlsx",
	}
	for in, want := range good {
		got, err := NormalizeDriveName(in)
		if err != nil || got != want {
			t.Errorf("%q: want %q, got %q (%v)", in, want, got, err)
		}
	}
	for _, bad := range []string{"", "   ", ".", "..", "a/b", `a\b`, "tab\there", strings.Repeat("я", 256)} {
		if _, err := NormalizeDriveName(bad); !errors.Is(err, ErrDriveBadName) {
			t.Errorf("%q must be rejected, got %v", bad, err)
		}
	}
}

func TestSanitizeDriveFileName(t *testing.T) {
	cases := map[string]string{
		`C:\Users\me\Паспорт.pdf`: "Паспорт.pdf",
		"../../etc/passwd":        "passwd",
		"":                        "file",
		"..":                      "file",
		"name\x00.txt":            "name.txt",
	}
	for in, want := range cases {
		if got := SanitizeDriveFileName(in); got != want {
			t.Errorf("%q: want %q, got %q", in, want, got)
		}
	}
	long := strings.Repeat("я", 300) + ".docx"
	got := SanitizeDriveFileName(long)
	if !strings.HasSuffix(got, ".docx") || len([]rune(got)) != driveMaxNameRunes {
		t.Errorf("long name must be cut to %d runes keeping extension, got %d runes", driveMaxNameRunes, len([]rune(got)))
	}
}

func TestNumberedDriveName(t *testing.T) {
	cases := []struct {
		name    string
		attempt int
		want    string
	}{
		{"отчёт.pdf", 1, "отчёт.pdf"},
		{"отчёт.pdf", 2, "отчёт (2).pdf"},
		{"архив.tar.gz", 3, "архив.tar (3).gz"},
		{"README", 2, "README (2)"},
		{".env", 2, ".env (2)"},
	}
	for _, c := range cases {
		if got := numberedDriveName(c.name, c.attempt); got != c.want {
			t.Errorf("%q #%d: want %q, got %q", c.name, c.attempt, c.want, got)
		}
	}
}

func TestDrivePreviewKind(t *testing.T) {
	cases := []struct {
		name, mime string
		office     bool
		want       string
	}{
		{"скан.pdf", "application/octet-stream", false, "pdf"},
		{"фото.jpg", "image/jpeg", false, "image"},
		{"логотип.svg", "image/svg+xml", false, ""}, // SVG может нести скрипты
		{"ролик.mp4", "video/mp4", false, "video"},
		{"звонок.mp3", "audio/mpeg", false, "audio"},
		{"выгрузка.csv", "text/csv", false, "text"},
		{"договор.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true, "office"},
		{"договор.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", false, ""},
		{"архив.zip", "application/zip", true, ""},
		{"без-расширения", "text/plain; charset=utf-8", false, "text"},
	}
	for _, c := range cases {
		if got := DrivePreviewKind(c.name, c.mime, c.office); got != c.want {
			t.Errorf("%s (%s, office=%v): want %q, got %q", c.name, c.mime, c.office, c.want, got)
		}
	}
}

// TestServedContentType — при показе в браузере разметка и скрипты отдаются
// как текст, а офисные форматы (в их MIME тоже есть «xml») остаются собой.
func TestServedContentType(t *testing.T) {
	const textPlain = "text/plain; charset=utf-8"
	cases := []struct {
		name, stored string
		inline       bool
		want         string
	}{
		{"page.html", "text/html", true, textPlain},
		{"logo.svg", "image/svg+xml", true, textPlain},
		{"app.js", "application/javascript", true, textPlain},
		{"data.json", "application/json", true, textPlain},
		{"Клиенты.csv", "text/csv; charset=utf-8", true, textPlain},
		// Регрессия: на Windows CSV сохранялся как «Excel» и уходил на скачивание.
		{"Клиенты.csv", "application/vnd.ms-excel", true, textPlain},
		{"scan.pdf", "application/pdf", true, "application/pdf"},
		{"photo.png", "image/png", true, "image/png"},
		{"report.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", true, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"page.html", "text/html", false, "text/html"}, // скачивание отдаёт как есть
		{"blob", "", false, "application/octet-stream"},
	}
	for _, c := range cases {
		if got := servedContentType(c.name, c.stored, c.inline); got != c.want {
			t.Errorf("%s (%q) inline=%v: want %q, got %q", c.name, c.stored, c.inline, c.want, got)
		}
	}
}

// TestDetectDriveContentTypeIsOSIndependent — тип частых форматов не должен
// зависеть от реестра Windows или /etc/mime.types в контейнере.
func TestDetectDriveContentTypeIsOSIndependent(t *testing.T) {
	cases := map[string]string{
		"Клиенты.csv":  "text/csv; charset=utf-8",
		"Договор.docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"Отчёт.XLSX":   "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"видео.MP4":    "video/mp4",
	}
	for name, want := range cases {
		if got := DetectDriveContentType(name, "application/octet-stream"); got != want {
			t.Errorf("%s: want %q, got %q", name, want, got)
		}
	}
}

func TestDetectDriveContentTypePrefersExtension(t *testing.T) {
	if got := DetectDriveContentType("скан.pdf", "application/octet-stream"); got != "application/pdf" {
		t.Fatalf("browser octet-stream must not override .pdf, got %q", got)
	}
	if got := DetectDriveContentType("без-расширения", ""); got != "application/octet-stream" {
		t.Fatalf("unknown must fall back to octet-stream, got %q", got)
	}
}

// TestDriveManageOnlyForAdmin — управлять хранилищем может только админ;
// руководство и ОКК видят лишь то, что им открыли.
func TestDriveManageOnlyForAdmin(t *testing.T) {
	if !(DriveActor{UserID: 1, RoleID: authz.RoleSystemAdmin}).canManage() {
		t.Error("admin must manage drive")
	}
	for _, role := range []int{authz.RoleManagement, authz.RoleControl, authz.RoleSales, authz.RoleVisa, authz.RoleHR, authz.RoleLegal, authz.RolePartner} {
		if (DriveActor{UserID: 1, RoleID: role}).canManage() {
			t.Errorf("role %d must not manage drive", role)
		}
	}
}

func TestNewDriveObjectKey(t *testing.T) {
	now := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	key, err := newDriveObjectKey(now, "Паспорт Иванова.PDF")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "drive/2026/09/") || !strings.HasSuffix(key, ".pdf") {
		t.Fatalf("unexpected key %q", key)
	}
	if strings.Contains(key, "Паспорт") {
		t.Fatalf("user file name must not leak into object key: %q", key)
	}
}
