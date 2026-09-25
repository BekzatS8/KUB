package storage

import (
	"fmt"
	"regexp"
	"strings"
)

// Разделение общего S3-бакета между компаниями.
//
// Несколько инсталляций CRM (KUB и другие компании) пишут в один бакет. Ключи
// объектов строятся из порядковых номеров базы — pdf/contract_deal_12.pdf,
// versions/5/v2_5.pdf, avatars/users/7/…, drive-previews/5.pdf, — а у каждой
// компании своя база с нумерацией от единицы. Без разделения договор №12
// одной компании перезаписал бы договор №12 другой, и её сотрудники увидели
// бы чужие документы. Поэтому все ключи компании живут под её префиксом:
// kub/pdf/…, company2/pdf/….

// LegacyTopLevelFolders — корневые папки, куда система пишет файлы. До
// введения префиксов данные KUB лежат в корне бакета именно в них.
//
// Список нужен дважды: код компании не может совпадать с такой папкой (иначе
// её файлы смешались бы с ещё не перенесёнными файлами в корне), а перенос
// существующих файлов трогает только эти папки — всё остальное в корне может
// оказаться чужим и остаётся на месте.
var LegacyTopLevelFolders = map[string]bool{
	"pdf":     true,
	"docx":    true,
	"excel":   true,
	"clients": true,
	"chat":    true,
	// Вложения чата с 10.12.2025 по 28.02.2026 сохранялись в messages/, потом
	// код перешёл на chat/. Старые вложения по-прежнему открываются по пути
	// из базы (attachments.storage_key), так что это живые данные.
	"messages":       true,
	"avatars":        true,
	"scoped":         true,
	"signatures":     true,
	"versions":       true,
	"drive":          true,
	"drive-previews": true,
}

var companyPrefixPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ValidateCompanyPrefix проверяет код компании для префикса в бакете.
// Пустой префикс допустим — это прежнее поведение (всё в корне бакета).
func ValidateCompanyPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if !companyPrefixPattern.MatchString(prefix) {
		return fmt.Errorf("s3 prefix %q: допустимы строчные латинские буквы, цифры и дефис, до 63 символов, например \"kub\"", prefix)
	}
	if LegacyTopLevelFolders[prefix] {
		return fmt.Errorf("s3 prefix %q совпадает с системной папкой в корне бакета — выберите код компании, например \"kub\"", prefix)
	}
	return nil
}

// NormalizeCompanyPrefix убирает пробелы и крайние слэши: «/kub/» → «kub».
func NormalizeCompanyPrefix(prefix string) string {
	return strings.Trim(strings.TrimSpace(prefix), "/")
}

// prefixedKey — ключ объекта с учётом префикса компании.
//
// Без префикса ключ возвращается байт в байт как раньше — поведение
// инсталляции, не включившей разделение, не меняется. С префиксом ведущий
// слэш отбрасывается (часть кода хранит пути вида «/pdf/x.pdf»), иначе
// получилось бы «kub//pdf/x.pdf»; перенос существующих файлов нормализует
// ключи так же.
func prefixedKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "/" + strings.TrimLeft(key, "/")
}
