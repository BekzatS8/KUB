package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

type dtSeekCloser struct{ *bytes.Reader }

func (dtSeekCloser) Close() error { return nil }

// dtStore имитирует S3: файл лежит по нормализованному ключу «pdf/x.pdf».
type dtStore struct {
	files    map[string][]byte
	openKeys []string
}

func (d *dtStore) Save(context.Context, io.Reader, string) error { return nil }
func (d *dtStore) Delete(context.Context, string) error          { return nil }
func (d *dtStore) Open(_ context.Context, key string) (io.ReadSeekCloser, int64, error) {
	d.openKeys = append(d.openKeys, key)
	b, ok := d.files[key]
	if !ok {
		return nil, 0, errors.New("NoSuchKey")
	}
	return dtSeekCloser{bytes.NewReader(b)}, int64(len(b)), nil
}

// downloadToTemp должен находить файл в S3 при ЛЮБОМ формате пути из БД:
// «/pdf/x.pdf», «files/pdf/x.pdf», «/files/pdf/x.pdf», «pdf/x.pdf».
// Это и был баг подписи: FinalizeSignedArtifact передавал сырой путь.
func TestDownloadToTempNormalizesKeyForStore(t *testing.T) {
	store := &dtStore{files: map[string][]byte{"pdf/x.pdf": []byte("%PDF")}}
	svc := &DocumentService{FilesRoot: "/nonexistent"}
	svc.SetStore(store)

	for _, raw := range []string{
		"/pdf/x.pdf",
		"files/pdf/x.pdf",
		"/files/pdf/x.pdf",
		"pdf/x.pdf",
		"\\pdf\\x.pdf",
	} {
		local, cleanup, err := svc.downloadToTemp(raw)
		if err != nil {
			t.Fatalf("downloadToTemp(%q): %v", raw, err)
		}
		data, _ := os.ReadFile(local)
		if string(data) != "%PDF" {
			t.Fatalf("downloadToTemp(%q): содержимое %q", raw, data)
		}
		cleanup()
	}
	// каждый вызов должен был спросить у хранилища именно «pdf/x.pdf»
	for _, k := range store.openKeys {
		if k != "pdf/x.pdf" {
			t.Fatalf("ключ хранилища не нормализован: %q", k)
		}
	}
}
