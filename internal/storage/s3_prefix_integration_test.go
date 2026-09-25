package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
)

// Интеграционный тест разделения бакета по компаниям на настоящем S3
// (MinIO). Проверяет ровно тот сценарий, ради которого сделан префикс: две
// компании кладут в один бакет файл с одинаковым ключом (номера в базах
// совпадают) — и каждая видит только свой.
//
// Запуск (нужен S3 с правом создавать бакеты, например локальный MinIO):
//
//	S3_TEST_ENDPOINT=localhost:9000 S3_TEST_ACCESS_KEY=... S3_TEST_SECRET_KEY=... \
//	  go test ./internal/storage -run S3Prefix
//
// Тест создаёт временный бакет и удаляет его в конце.
func TestS3PrefixIntegration(t *testing.T) {
	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3_TEST_ENDPOINT не задан — интеграционный тест S3 пропущен")
	}
	access, secret := os.Getenv("S3_TEST_ACCESS_KEY"), os.Getenv("S3_TEST_SECRET_KEY")
	ctx := context.Background()

	rnd := make([]byte, 4)
	_, _ = rand.Read(rnd)
	bucket := "kubtest-" + hex.EncodeToString(rnd)

	raw, err := NewS3Storage(endpoint, "", bucket, access, secret, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatalf("make bucket: %v", err)
	}
	defer func() {
		for o := range raw.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			_ = raw.client.RemoveObject(ctx, bucket, o.Key, minio.RemoveObjectOptions{})
		}
		_ = raw.client.RemoveBucket(ctx, bucket)
	}()

	put := func(key, body string) {
		t.Helper()
		if err := raw.Save(ctx, strings.NewReader(body), key); err != nil {
			t.Fatalf("put %s: %v", key, err)
		}
	}
	read := func(st Storage, key string) (string, error) {
		r, _, err := st.Open(ctx, key)
		if err != nil {
			return "", err
		}
		defer r.Close()
		b, err := io.ReadAll(r)
		return string(b), err
	}
	exists := func(key string) bool {
		_, err := raw.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
		return err == nil
	}

	// Состояние «до»: файлы KUB в корне + чужая компания уже под префиксом +
	// посторонний файл в корне.
	put("pdf/contract_deal_12.pdf", "KUB договор 12")
	put("clients/1/passport/a.jpg", "KUB паспорт")
	put("drive/2026/09/f.txt", "KUB хранилище")
	put("drive-previews/5.pdf", "KUB превью 5")
	put("company2/pdf/contract_deal_12.pdf", "ЧУЖОЙ договор 12")
	put("readme.txt", "посторонний файл")

	t.Run("план ничего не меняет", func(t *testing.T) {
		rep, err := raw.MigrateToPrefix(ctx, PrefixMigrationOptions{To: "kub"})
		if err != nil {
			t.Fatal(err)
		}
		if rep.Planned != 4 || rep.Copied != 0 {
			t.Fatalf("expected 4 planned, 0 copied: %+v", rep)
		}
		if rep.Untouched["company2"] != 1 || rep.Untouched[""] != 1 {
			t.Fatalf("foreign data must be reported untouched: %+v", rep.Untouched)
		}
		if exists("kub/pdf/contract_deal_12.pdf") {
			t.Fatal("dry run must not copy")
		}
	})

	t.Run("копирование, исходники остаются", func(t *testing.T) {
		rep, err := raw.MigrateToPrefix(ctx, PrefixMigrationOptions{To: "kub", Apply: true})
		if err != nil || rep.Copied != 4 || rep.Failed != 0 {
			t.Fatalf("copy: %+v %v", rep, err)
		}
		if !exists("pdf/contract_deal_12.pdf") {
			t.Fatal("sources must stay without -delete-source")
		}
		if !exists("company2/pdf/contract_deal_12.pdf") || !exists("readme.txt") {
			t.Fatal("foreign objects must not be touched")
		}
	})

	t.Run("повторный запуск безопасен", func(t *testing.T) {
		rep, err := raw.MigrateToPrefix(ctx, PrefixMigrationOptions{To: "kub", Apply: true})
		if err != nil || rep.Copied != 0 || rep.AlreadyCopied != 4 {
			t.Fatalf("rerun must copy nothing: %+v %v", rep, err)
		}
	})

	t.Run("компании изолированы при одинаковых ключах", func(t *testing.T) {
		kub, err := NewS3Storage(endpoint, "", bucket, access, secret, false, "kub")
		if err != nil {
			t.Fatal(err)
		}
		other, err := NewS3Storage(endpoint, "", bucket, access, secret, false, "company2")
		if err != nil {
			t.Fatal(err)
		}
		// Один и тот же ключ из базы — у каждой компании свой файл.
		if got, _ := read(kub, "pdf/contract_deal_12.pdf"); got != "KUB договор 12" {
			t.Fatalf("kub must read its own contract, got %q", got)
		}
		if got, _ := read(other, "pdf/contract_deal_12.pdf"); got != "ЧУЖОЙ договор 12" {
			t.Fatalf("company2 must read its own contract, got %q", got)
		}
		// Старый путь с ведущим слэшем тоже находится.
		if got, _ := read(kub, "/drive-previews/5.pdf"); got != "KUB превью 5" {
			t.Fatalf("leading slash key must resolve under prefix, got %q", got)
		}
		// Запись и удаление — только внутри своего префикса.
		if err := SaveWithSize(ctx, kub, bytes.NewReader([]byte("новый")), "drive/new.txt", 10, "text/plain"); err != nil {
			t.Fatal(err)
		}
		if !exists("kub/drive/new.txt") || exists("drive/new.txt") {
			t.Fatal("write must land under kub/ only")
		}
		if err := kub.Delete(ctx, "drive/new.txt"); err != nil || exists("kub/drive/new.txt") {
			t.Fatalf("delete must remove kub/drive/new.txt: %v", err)
		}
	})

	t.Run("удаление исходников после проверки", func(t *testing.T) {
		if _, err := raw.MigrateToPrefix(ctx, PrefixMigrationOptions{To: "kub", DeleteSource: true}); err == nil {
			t.Fatal("delete-source without apply must be rejected")
		}
		rep, err := raw.MigrateToPrefix(ctx, PrefixMigrationOptions{To: "kub", Apply: true, DeleteSource: true})
		if err != nil || rep.Deleted != 4 || rep.Failed != 0 {
			t.Fatalf("delete sources: %+v %v", rep, err)
		}
		if exists("pdf/contract_deal_12.pdf") {
			t.Fatal("source must be removed")
		}
		if !exists("kub/pdf/contract_deal_12.pdf") {
			t.Fatal("copy must survive source removal")
		}
		if !exists("company2/pdf/contract_deal_12.pdf") || !exists("readme.txt") {
			t.Fatal("foreign objects must survive")
		}
	})
}
