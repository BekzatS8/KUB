// Команда s3-company-prefix переносит файлы инсталляции из корня общего
// S3-бакета под префикс компании (pdf/… → kub/pdf/…).
//
// Нужна один раз для KUB: до введения префиксов все его файлы лежали в корне
// бакета. Доступы к S3 берутся из того же конфига и переменных окружения, что и
// у приложения (CONFIG_PATH, S3_*) — секреты в командной строке не нужны.
//
// По умолчанию ничего не меняет, только показывает план. Порядок переключения
// без простоя описан в docs/storage-company-prefix.md.
//
//	s3-company-prefix -to kub                   # план: что и сколько переносится
//	s3-company-prefix -to kub -apply            # копирование (исходники остаются)
//	s3-company-prefix -to kub -apply -delete-source   # удалить исходники после проверки
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"turcompany/internal/config"
	"turcompany/internal/storage"
)

func main() {
	to := flag.String("to", "", "код компании, под который переносить файлы (по умолчанию — S3_PREFIX из конфига)")
	apply := flag.Bool("apply", false, "выполнить копирование; без флага — только план")
	deleteSource := flag.Bool("delete-source", false, "удалить исходники после проверенного копирования (только с -apply)")
	extra := flag.String("extra-folders", "", "дополнительные корневые папки через запятую, тоже принадлежащие этой инсталляции")
	verbose := flag.Bool("v", false, "печатать каждый объект")
	flag.Parse()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("конфиг: %v", err)
	}
	if !cfg.S3.Enabled {
		log.Fatal("S3 не включён в конфиге — переносить нечего")
	}
	target := strings.TrimSpace(*to)
	if target == "" {
		target = cfg.S3.Prefix
	}
	if target == "" {
		log.Fatal("укажите код компании: -to kub (или задайте S3_PREFIX)")
	}

	// Хранилище без префикса: перенос работает с бакетом напрямую.
	st, err := storage.NewS3Storage(cfg.S3.Endpoint, cfg.S3.Region, cfg.S3.Bucket, cfg.S3.AccessKey, cfg.S3.SecretKey, cfg.S3.UseSSL, "")
	if err != nil {
		log.Fatalf("S3: %v", err)
	}

	var folders []string
	for _, f := range strings.Split(*extra, ",") {
		if f = strings.TrimSpace(f); f != "" {
			folders = append(folders, f)
		}
	}

	mode := "ПЛАН (ничего не меняется, для выполнения добавьте -apply)"
	if *apply {
		mode = "КОПИРОВАНИЕ"
		if *deleteSource {
			mode = "КОПИРОВАНИЕ С УДАЛЕНИЕМ ИСХОДНИКОВ"
		}
	}
	fmt.Printf("Бакет: %s @ %s\nЦель:  %s/…\nРежим: %s\n\n", cfg.S3.Bucket, cfg.S3.Endpoint, target, mode)

	report, err := st.MigrateToPrefix(context.Background(), storage.PrefixMigrationOptions{
		To:           target,
		ExtraFolders: folders,
		Apply:        *apply,
		DeleteSource: *deleteSource,
		Progress: func(action, src, dst string) {
			if *verbose {
				fmt.Printf("  %-8s %s → %s\n", action, src, dst)
			}
		},
	})
	if report != nil {
		printReport(report, target, *apply)
	}
	if err != nil {
		log.Fatalf("перенос прерван: %v", err)
	}
	if report.Failed > 0 {
		os.Exit(1)
	}
}

func printReport(r *storage.PrefixMigrationReport, target string, applied bool) {
	row := func(label string, value any) { fmt.Printf("%-30s %v\n", label, value) }
	fmt.Println()
	row("Просмотрено объектов:", r.Scanned)
	row("Уже лежат под "+target+"/:", r.AlreadyPrefixed)
	row("К переносу:", fmt.Sprintf("%d (%s)", r.Planned, humanBytes(r.PlannedBytes)))
	if applied {
		row("  скопировано:", r.Copied)
		row("  копия уже была:", r.AlreadyCopied)
		row("  исходников удалено:", r.Deleted)
		row("  ошибок:", r.Failed)
	}
	if len(r.Untouched) > 0 {
		fmt.Println("\nОставлены на месте (не системные папки — возможно, чужие файлы):")
		keys := make([]string, 0, len(r.Untouched))
		for k := range r.Untouched {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			name := k + "/"
			if k == "" {
				name = "(файлы в корне бакета)"
			}
			fmt.Printf("  %-28s %d\n", name, r.Untouched[k])
		}
		fmt.Println("Если какая-то из этих папок принадлежит этой инсталляции — добавьте её через -extra-folders.")
	}
	for _, e := range r.Errors {
		fmt.Println("ОШИБКА:", e)
	}
}

func humanBytes(n int64) string {
	units := []string{"Б", "КБ", "МБ", "ГБ", "ТБ"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
