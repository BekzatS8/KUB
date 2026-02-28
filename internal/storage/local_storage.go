package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	root string
}

func NewLocalStorage(root string) *LocalStorage {
	return &LocalStorage{root: root}
}

func (s *LocalStorage) Save(_ context.Context, reader io.Reader, key string) error {
	fullPath, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, reader)
	return err
}

func (s *LocalStorage) Open(_ context.Context, key string) (io.ReadSeekCloser, int64, error) {
	fullPath, err := s.resolvePath(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}

func (s *LocalStorage) Delete(_ context.Context, key string) error {
	fullPath, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStorage) resolvePath(key string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(key))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") || strings.Contains(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid storage key")
	}
	full := filepath.Join(s.root, clean)
	rootAbs, _ := filepath.Abs(s.root)
	fullAbs, _ := filepath.Abs(full)
	if !strings.HasPrefix(fullAbs, rootAbs+string(filepath.Separator)) && fullAbs != rootAbs {
		return "", fmt.Errorf("invalid storage key")
	}
	return full, nil
}
